package main

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"time"
)

type Event struct {
	ID        string
	UserID    int
	Action    string
	Timestamp time.Time
}

func handleEvent(event Event, wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Printf("Начинаем обработку: %s\n", event.Action)
	time.Sleep(2 * time.Second) // имитация долгой работы
	fmt.Printf("Обработано: %s\n", event.Action)
}

func generateEvent(userID int) Event {
	actions := []string{"вошел", "вышел", "купил", "посмотрел"}
	return Event{
		ID:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		UserID:    userID,
		Action:    actions[rand.IntN(len(actions))],
		Timestamp: time.Now(),
	}
}

func main() {
	fmt.Println("Программа запущена")

	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < 5; i++ {
		event := generateEvent(1)

		wg.Add(1)
		go handleEvent(event, &wg)

		fmt.Println("Событие отправлено в обработку")
		time.Sleep(500 * time.Millisecond)
	}

	wg.Wait()

	fmt.Printf("Всего затрачено времени: %v\n", time.Since(start))
}
