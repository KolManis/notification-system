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

func worker(id int, events <-chan Event, wg *sync.WaitGroup) {
	for event := range events {
		fmt.Printf("  Воркер %d: начал обработку %s\n", id, event.Action)
		time.Sleep(2 * time.Second) // работаем
		fmt.Printf("  Воркер %d: закончил %s\n", id, event.Action)
		wg.Done()
	}
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
	eventChannel := make(chan Event, 10)
	defer close(eventChannel)
	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		go worker(i+1, eventChannel, &wg)
	}

	fmt.Println("Запущено 3 воркера")

	for i := 0; i < 5; i++ {
		event := generateEvent(1)

		wg.Add(1)
		eventChannel <- event

		fmt.Printf(" Событие %d в очереди\n", i+1)
		time.Sleep(300 * time.Millisecond)
	}

	wg.Wait()

	fmt.Println("Все события обработаны")
}
