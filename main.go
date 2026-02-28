package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	amqp "github.com/rabbitmq/amqp091-go" // Официальный клиент RabbitMQ
)

// Event - структура события, которую мы будем отправлять
type Event struct {
	ID        string                 `json:"id"`
	UserID    string                 `json:"user_id"`
	Action    string                 `json:"action"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// RabbitMQClient - структура для работы с RabbitMQ
type RabbitMQClient struct {
	conn    *amqp.Connection // соединение с сервером
	channel *amqp.Channel    // канал для обмена данными
	queue   amqp.Queue       // очередь, куда складываем сообщения
}

// NewRabbitMQClient - создает нового клиента RabbitMQ
func NewRabbitMQClient() (*RabbitMQClient, error) {
	// ПОДКЛЮЧЕНИЕ К RABBITMQ
	// amqp://guest:guest@localhost:5672/ - это URL для подключения
	// guest:guest - логин:пароль (по умолчанию)
	// localhost:5672 - адрес сервера и порт
	log.Println(" Подключаемся к RabbitMQ...")
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к RabbitMQ: %w", err)
	}
	log.Println(" Подключено к RabbitMQ")

	// СОЗДАЕМ КАНАЛ
	// Канал - это виртуальное соединение внутри физического соединения
	// Через каналы мы отправляем и получаем сообщения
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ошибка создания канала: %w", err)
	}

	// ОБЪЯВЛЯЕМ ОЧЕРЕДЬ (СОЗДАЕМ ЕЕ, ЕСЛИ НЕТ)
	// QueueDeclare - создает очередь, если ее нет, или возвращает существующую
	// Параметры:
	// - "events_queue" - имя очереди
	// - true - durable (сохранять на диск) - ЭТО ВАЖНО ДЛЯ СОХРАННОСТИ!
	// - false - delete when unused (не удалять, когда не используется)
	// - false - exclusive (не эксклюзивная, другие клиенты тоже могут подключаться)
	// - false - no-wait (не ждать подтверждения от сервера)
	// - nil - аргументы (доп. настройки)
	q, err := ch.QueueDeclare(
		"events_queue", // name
		true,           // durable 📌 СОХРАНЯТЬ НА ДИСК!
		false,          // delete when unused
		false,          // exclusive
		false,          // no-wait
		nil,            // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("ошибка создания очереди: %w", err)
	}

	log.Printf(" Очередь '%s' готова (сообщений в очереди: %d)", q.Name, q.Messages)

	return &RabbitMQClient{
		conn:    conn,
		channel: ch,
		queue:   q,
	}, nil
}

// Publish - отправляет событие в RabbitMQ
func (c *RabbitMQClient) Publish(event Event) error {
	// Превращаем структуру в JSON (байты)
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("ошибка кодирования JSON: %w", err)
	}

	// Публикуем сообщение в очередь
	// Параметры:
	// - "" - exchange (пустая строка = exchange по умолчанию)
	// - c.queue.Name - routing key (имя очереди)
	// - false - mandatory (если true, сервер вернет ошибку, если очередь не найдена)
	// - false - immediate (устаревший параметр)
	// - amqp.Publishing - само сообщение
	err = c.channel.Publish(
		"",           // exchange (по умолчанию)
		c.queue.Name, // routing key (имя очереди)
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType:  "application/json", // тип содержимого
			Body:         body,               // само сообщение (JSON)
			MessageId:    event.ID,           // ID сообщения
			Timestamp:    time.Now(),         // время отправки
			DeliveryMode: amqp.Persistent,    // PERSISTENT - сохранять на диск!
			// Это гарантирует, что сообщение не пропадет при перезапуске сервера
		})

	if err != nil {
		return fmt.Errorf("ошибка публикации: %w", err)
	}

	log.Printf(" Событие %s отправлено в очередь", event.ID[:8])
	return nil
}

// Consume - запускает получение сообщений из очереди
func (c *RabbitMQClient) Consume(workerID int) error {
	// Начинаем получать сообщения из очереди
	// Параметры:
	// - c.queue.Name - имя очереди
	// - fmt.Sprintf("worker_%d", workerID) - имя потребителя (для отладки)
	// - false - auto-ack (false = ручное подтверждение!)
	// - false - exclusive (не эксклюзивный)
	// - false - no-local (не получать сообщения от себя же)
	// - false - no-wait (не ждать)
	// - nil - аргументы
	msgs, err := c.channel.Consume(
		c.queue.Name,                       // очередь
		fmt.Sprintf("worker_%d", workerID), // consumer tag (имя потребителя)
		false,                              // auto-ack
		false,                              // exclusive
		false,                              // no-local
		false,                              // no-wait
		nil,                                // args
	)
	if err != nil {
		return fmt.Errorf("ошибка регистрации потребителя: %w", err)
	}

	log.Printf(" Воркер %d начал слушать очередь '%s'", workerID, c.queue.Name)

	// Бесконечный цикл получения сообщений
	// msgs - это канал, в который приходят сообщения
	for msg := range msgs {
		// msg - это amqp.Delivery (полученное сообщение)

		// Парсим JSON обратно в структуру Event
		var event Event
		err := json.Unmarshal(msg.Body, &event)
		if err != nil {
			log.Printf(" Воркер %d: ошибка парсинга: %v", workerID, err)
			// Nack - Negative Acknowledgement (отрицательное подтверждение)
			// false - не переотправлять множественные сообщения
			// true - вернуть сообщение в очередь (requeue)
			msg.Nack(false, true)
			continue
		}

		log.Printf("   Воркер %d: получил событие: %s от пользователя %s",
			workerID, event.Action, event.UserID)

		// Имитируем обработку (разное время для разных действий)
		switch event.Action {
		case "purchase":
			time.Sleep(3 * time.Second)
		case "login":
			time.Sleep(1 * time.Second)
		default:
			time.Sleep(2 * time.Second)
		}

		log.Printf("     Воркер %d: обработал событие %s", workerID, event.ID[:8])

		// ПОДТВЕРЖДАЕМ успешную обработку!
		// Если мы не вызовем Ack, сообщение останется в очереди
		// и будет отправлено снова другому потребителю (или этому же при перезапуске)
		// false - не подтверждать множественные сообщения

		msg.Ack(false)
	}

	return nil
}

// Close - закрывает соединения
func (c *RabbitMQClient) Close() {
	log.Println(" Закрываем соединения с RabbitMQ...")
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
	log.Println(" Соединения закрыты")
}

// HTTP обработчик
func createEventHandler(rmq *RabbitMQClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var event Event
		err := json.NewDecoder(r.Body).Decode(&event)
		if err != nil {
			http.Error(w, "Неверный формат JSON", http.StatusBadRequest)
			return
		}

		// Валидация
		if event.UserID == "" {
			http.Error(w, "user_id обязателен", http.StatusBadRequest)
			return
		}
		if event.Action == "" {
			http.Error(w, "action обязателен", http.StatusBadRequest)
			return
		}

		// Генерируем ID и время
		event.ID = uuid.New().String()
		event.Timestamp = time.Now()

		// Отправляем в RabbitMQ
		err = rmq.Publish(event)
		if err != nil {
			log.Printf(" Ошибка публикации: %v", err)
			http.Error(w, "Ошибка отправки в очередь", http.StatusInternalServerError)
			return
		}

		// Отвечаем клиенту
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "accepted",
			"id":     event.ID,
		})
	}
}

func main() {
	//  ПОДКЛЮЧАЕМСЯ К RABBITMQ
	rmq, err := NewRabbitMQClient()
	if err != nil {
		log.Fatal(" Не удалось подключиться к RabbitMQ:", err)
	}
	defer rmq.Close() // при выходе из программы закрываем соединения

	//  ЗАПУСКАЕМ ВОРКЕРОВ (ПОТРЕБИТЕЛЕЙ)
	// Запускаем 3 воркера в отдельных горутинах
	for i := 1; i <= 3; i++ {
		go func(workerID int) {
			// Каждый воркер начинает получать сообщения
			if err := rmq.Consume(workerID); err != nil {
				log.Printf(" Воркер %d ошибка: %v", workerID, err)
			}
		}(i)
	}

	//  НАСТРАИВАЕМ HTTP СЕРВЕР
	r := mux.NewRouter()

	// Эндпоинт для приема событий
	r.HandleFunc("/api/events", createEventHandler(rmq)).Methods("POST")

	// Эндпоинт для проверки здоровья
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()

		// Проверяем, жива ли еще связь с RabbitMQ
		if rmq.conn.IsClosed() {
			http.Error(w, "RabbitMQ disconnected", http.StatusServiceUnavailable)
			return
		}

		healthStatus := map[string]interface{}{
			"status":   "ok",
			"time":     time.Now().Format(time.RFC3339),
			"version":  "1.0.0",
			"rabbitmq": "connected",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(healthStatus)
	})

	// Простая HTML страница для тестирования
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		html := `<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>Тестер RabbitMQ</title>
	<style>
		body { font-family: Arial; margin: 40px; }
		form { background: #f5f5f5; padding: 20px; max-width: 400px; }
		input, select, button { margin: 10px 0; padding: 8px; width: 100%; }
		pre { background: #eee; padding: 10px; }
	</style>
</head>
<body>
	<h2>📨 Отправить событие в RabbitMQ</h2>
	<form id="eventForm">
		<label>User ID:</label>
		<input type="text" id="userId" value="user123">
		
		<label>Action:</label>
		<select id="action">
			<option value="login">Вход (login)</option>
			<option value="logout">Выход (logout)</option>
			<option value="purchase">Покупка (purchase)</option>
			<option value="view">Просмотр (view)</option>
		</select>
		
		<button type="submit">Отправить</button>
	</form>
	<div id="result"></div>
	
	<script>
		document.getElementById('eventForm').onsubmit = async (e) => {
			e.preventDefault();
			const response = await fetch('/api/events', {
				method: 'POST',
				headers: {'Content-Type': 'application/json'},
				body: JSON.stringify({
					user_id: document.getElementById('userId').value,
					action: document.getElementById('action').value
				})
			});
			const result = await response.json();
			document.getElementById('result').innerHTML = 
				'<pre>' + JSON.stringify(result, null, 2) + '</pre>';
		}
	</script>
</body>
</html>`

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, html)
	})

	// 4️⃣ ЗАПУСКАЕМ СЕРВЕР
	log.Println(" HTTP сервер запущен на :8080")
	log.Println(" Открой http://localhost:8080 в браузере")
	log.Fatal(http.ListenAndServe(":8080", r))
}
