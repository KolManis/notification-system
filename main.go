package main

import (
	"fmt"
	"math/rand/v2"
	"time"
)

type Event struct {
	ID        string
	UserID    int
	Action    string
	Timestamp time.Time
}

func handleEvent(event Event) {
	fmt.Printf("[%s] Полльзователь %d: %s\n",
		event.Timestamp.Format("15:04:05"),
		event.UserID,
		event.Action)
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
	fmt.Println("🔧 Программа запущена")

	for i := 0; i < 50; i++ {
		event := generateEvent(1)
		handleEvent(event)
		time.Sleep(1 * time.Second)
	}

	fmt.Println("✅ Программа завершена")
}
