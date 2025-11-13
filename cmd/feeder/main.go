package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"exdexTiker/internal/kafka"
	"exdexTiker/utils/cache"
	"exdexTiker/utils/database"
)

type Price struct {
	Symbol string  `json:"symbol"`
	Price  float64 `json:"price"`
}

func main() {
	ctx := context.Background()
	conn := database.ConnectPostgres()
	defer conn.Close(ctx)

	rdb := cache.ConnectRedis()
	defer rdb.Close()

	database.AutoMigrate(conn)
	kafkaWriter := kafka.NewWriter("localhost:9092", "prices")
	defer kafkaWriter.Close()
	client := &http.Client{Timeout: 10 * time.Second}
	url := "https://api.binance.com/api/v3/ticker/price"

	for {
		start := time.Now()

		resp, err := client.Get(url)
		if err != nil {
			log.Println("⚠️ [FETCH ERROR] Failed to fetch:", err)
			time.Sleep(time.Second)
			continue
		}

		var raw []map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			log.Println("⚠️ [DECODE ERROR] Failed to decode JSON:", err)
			resp.Body.Close()
			time.Sleep(time.Second)
			continue
		}
		resp.Body.Close()

		batch := &pgx.Batch{}
		count := 0

		for _, p := range raw {
			symbol := p["symbol"]
			priceStr := p["price"]

			base, quote := splitSymbol(symbol)
			if quote == "" {
				continue
			}

			priceVal, err := strconv.ParseFloat(priceStr, 64)
			if err != nil {
				continue
			}

			var prevPrice, change, changePercent float64
			_ = conn.QueryRow(ctx,
				`SELECT price FROM exdex_token WHERE base_currency=$1 AND quote_currency=$2`,
				base, quote).Scan(&prevPrice)

			if prevPrice != 0 {
				change = priceVal - prevPrice
				changePercent = (change / prevPrice) * 100
			}

			// Prepare batch insert
			batch.Queue(`
				INSERT INTO exdex_token (
					base_currency, quote_currency, price, price_change, price_change_percent, updated_at
				)
				VALUES ($1,$2,$3,$4,$5,$6)
				ON CONFLICT (base_currency, quote_currency)
				DO UPDATE SET 
					price = EXCLUDED.price,
					price_change = EXCLUDED.price_change,
					price_change_percent = EXCLUDED.price_change_percent,
					updated_at = EXCLUDED.updated_at
			`, base, quote, priceVal, change, changePercent, time.Now())

			// 🔥 Save to Redis for fast access
			cacheKey := fmt.Sprintf("%s_%s", base, quote)
			cache.SavePrice(ctx, rdb, cacheKey, priceVal)

			// 🧩 Log this particular update line
			fmt.Printf("📥 [PG Insert/Update] %-10s %-6s => %.8f | Δ %.6f (%.3f%%)\n",
				base, quote, priceVal, change, changePercent)
			fmt.Printf("💾 [Redis Cache] Key: %s | Value: %.8f\n", cacheKey, priceVal)

			// inside your for loop after cache.SavePrice(...)
			// kafka.PublishPrice(ctx, kafkaWriter, cacheKey, priceVal)
			count++
		}
		cache.GetAllKeysValues(ctx, rdb)
		// Execute batch insert
		br := conn.SendBatch(ctx, batch)
		if err := br.Close(); err != nil {
			log.Println("⚠️ [BATCH ERROR] Batch insert error:", err)
		}

		fmt.Printf("\n✅ [CYCLE DONE] Upserted %d pairs & cached to Redis in %v\n", count, time.Since(start))
		fmt.Println("───────────────────────────────────────────────\n")

		time.Sleep(1 * time.Second)
	}
}

// splitSymbol dynamically detects quote currencies to avoid missing any coins.
func splitSymbol(symbol string) (string, string) {
	quotes := []string{
		"USDT", "BUSD", "USDC", "FDUSD", "TUSD", "BTC", "ETH", "BNB",
		"TRY", "BRL", "EUR", "GBP", "AUD", "ZAR", "JPY", "RUB", "UAH",
		"NGN", "VND", "IDR", "DAI", "TRX", "PAX", "FDUSD",
	}
	for _, q := range quotes {
		if len(symbol) > len(q) && symbol[len(symbol)-len(q):] == q {
			return symbol[:len(symbol)-len(q)], q
		}
	}

	return symbol, ""
}

// GetAllKeysValues lists all keys and their current values
