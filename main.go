package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type Event struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
}

var eventQueue = make(chan Event, 100)

// Обработчик HTTP для приема событий
func createEventHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var event Event

	err := json.NewDecoder(r.Body).Decode(&event)
	if err != nil {
		http.Error(w, "Неверный формат JSON", http.StatusBadRequest)
		return
	}

	event.ID = uuid.New().String()
	event.Timestamp = time.Now()

	// Отправляем в очередь (неблокирующая отправка)
	select {
	case eventQueue <- event:
		log.Printf("Событие добавлено в очередь: %+v", event)
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "accepted",
			"id":     event.ID,
		})
	default:
		http.Error(w, "Очередь переполнена", http.StatusServiceUnavailable)
	}
}

// Воркер для обработки событий
func startWorker(id int) {
	for event := range eventQueue {
		log.Printf(" Воркер %d: обрабатываю %s для пользователя %s",
			id, event.Action, event.UserID)

		time.Sleep(2 * time.Second)

		log.Printf("   Воркер %d: завершил обработку %s", id, event.ID[:8])
	}
}

func main() {
	for i := 1; i <= 3; i++ {
		go startWorker(i)
	}

	r := mux.NewRouter()
	r.HandleFunc("/api/events", createEventHandler).Methods("POST")
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		defer r.Body.Close()

		healthStatus := map[string]interface{}{
			"status":  "ok",
			"time":    time.Now().Format(time.RFC3339),
			"version": "1.0.0",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(healthStatus)
	})

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		html := `
        <!DOCTYPE html>
        <html>
        <head>
			<meta charset="UTF-8">	
			<title>Тестер событий</title>
		</head>
        <body>
            <h2>Отправить событие</h2>
            <form id="eventForm">
                User ID: <input type="text" id="userId" value="user123"><br>
                Action: 
                <select id="action">
                    <option>login</option>
                    <option>logout</option>
                    <option>purchase</option>
                    <option>view</option>
                </select><br>
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
        </html>
        `
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, html)
	})

	log.Println("Сервер запущен на :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
