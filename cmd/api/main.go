package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

var rdb *redis.Client

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins
}

// ✅ Connect to Redis
func connectRedis() *redis.Client {
	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	fmt.Println("✅ Connected to Redis")
	return rdb
}

// ✅ Fetch all Redis key-value pairs
func getAllKeysValues(ctx context.Context) (map[string]string, error) {
	result := make(map[string]string)

	keys, err := rdb.Keys(ctx, "*").Result()
	if err != nil {
		return nil, err
	}

	for _, key := range keys {
		val, err := rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		result[key] = val
	}
	return result, nil
}

// ✅ API 1: Get single symbol value
// GET /api/symbol?symbol=ZKC_BNB
func getSymbolHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		http.Error(w, "symbol query param required, e.g. /api/symbol?symbol=ZKC_BNB", http.StatusBadRequest)
		return
	}

	val, err := rdb.Get(ctx, symbol).Result()
	if err == redis.Nil {
		http.Error(w, "symbol not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]string{symbol: val}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ✅ API 2: Get all Redis data (REST)
// GET /api/all
func getAllHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	data, err := getAllKeysValues(ctx)
	if err != nil {
		http.Error(w, "failed to fetch data", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// ✅ API 3: WebSocket stream — /ws
func wsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("❌ WebSocket upgrade failed:", err)
		return
	}
	defer conn.Close()
	log.Println("✅ WebSocket client connected")

	for {
		data, err := getAllKeysValues(ctx)
		if err != nil {
			log.Println("❌ Error fetching Redis data:", err)
			continue
		}

		jsonData, _ := json.Marshal(data)
		err = conn.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			log.Println("⚠️ WebSocket client disconnected:", err)
			break
		}
		time.Sleep(1 * time.Second)
	}
}

func main() {
	rdb = connectRedis()
	defer rdb.Close()

	// Register routes
	http.HandleFunc("/api/symbol", getSymbolHandler)
	http.HandleFunc("/api/all", getAllHandler)
	http.HandleFunc("/ws", wsHandler)

	fmt.Println("🚀 Server running:")
	fmt.Println("   ➤ WebSocket: ws://localhost:8080/ws")
	fmt.Println("   ➤ REST API (single): http://localhost:8080/api/symbol?symbol=ZKC_BNB")
	fmt.Println("   ➤ REST API (all):    http://localhost:8080/api/all")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("❌ Server failed:", err)
	}
}
