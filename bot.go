package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/adshao/go-binance/v2"
	"github.com/joho/godotenv"
)

var client *binance.Client
var closes []float64 // Variable global para almacenar los precios de cierre

const (
	symbol        = "BTCUSDT"
	quantity      = 0.00130
	rsiPeriod     = 14
	rsiOverbought = 69.0
	rsiOversold   = 30.0
)

// Load environment variables from the .env file
func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
}

// Initialize the Binance client with API keys
func initBinanceClient() {
	apiKey := os.Getenv("BINANCE_API_KEY")
	apiSecret := os.Getenv("BINANCE_API_SECRET")
	client = binance.NewClient(apiKey, apiSecret)
}

// Calculate RSI based on closing prices using SMA
func calculateRSIWithSMA(closes []float64, period int) float64 {
	var gains, losses []float64

	for i := 1; i < len(closes); i++ {
		change := closes[i] - closes[i-1]
		if change > 0 {
			gains = append(gains, change)
			losses = append(losses, 0)
		} else {
			losses = append(losses, -change)
			gains = append(gains, 0)
		}
	}

	averageGain := sum(gains[:period]) / float64(period)
	averageLoss := sum(losses[:period]) / float64(period)

	for i := period; i < len(gains); i++ {
		averageGain = (averageGain*(float64(period-1)) + gains[i]) / float64(period)
		averageLoss = (averageLoss*(float64(period-1)) + losses[i]) / float64(period)
	}
	if averageLoss == 0 {
		return 100
	}

	rs := averageGain / averageLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// Helper function to sum a slice of float64
func sum(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}

// Check RSI and log the result
func checkRSIAndTrade(closes []float64) {
	rsi := calculateRSIWithSMA(closes, rsiPeriod)
	log.Printf("Current RSI: %.2f", rsi)

	if rsi <= rsiOversold {
		log.Println("RSI below 30. Executing buy.")
		trade(symbol, binance.SideTypeBuy)
	} else if rsi >= rsiOverbought {
		log.Println("RSI above 69. Executing sell.")
		trade(symbol, binance.SideTypeSell)
	}
}

// Perform trading operation (buy/sell)
func trade(symbol string, side binance.SideType) {
	// Aquí puedes implementar tu lógica de trading si es necesario
	log.Printf("Trade executed: %s %s", side, symbol)
}

// Get closing prices from the last 5-minute candles
func getHistoricalData(symbol string, limit int) ([]float64, error) {
	klines, err := client.NewKlinesService().Symbol(symbol).
		Interval("5m").
		Limit(100).
		Do(context.Background())
	if err != nil {
		return nil, err
	}

	var closes []float64
	for _, kline := range klines {
		closePrice, _ := strconv.ParseFloat(kline.Close, 64)
		closes = append(closes, closePrice)
	}
	return closes, nil
}


func main() {
	// Load environment variables and initialize Binance client
	loadEnv()
	initBinanceClient()

	// Obtener datos históricos para inicializar los precios de cierre
	var err error
	closes, err = getHistoricalData(symbol, rsiPeriod+1)
	if err != nil {
		log.Fatalf("Error fetching historical data: %v", err)
	}

	// Real-time price data using WebSocket
	wsKlineHandler := func(event *binance.WsKlineEvent) {
		// Solo añadir el precio de cierre cuando la vela se ha cerrado
		if event.Kline.IsFinal {
			closePrice, _ := strconv.ParseFloat(event.Kline.Close, 64)
			closes = append(closes, closePrice)

			// Maintain only the last rsiPeriod+1 closes
			if len(closes) > rsiPeriod+1 {
				closes = closes[1:]
			}

			// Calculate and check RSI if we have enough data points
			if len(closes) >= rsiPeriod+1 {
				checkRSIAndTrade(closes)
			}
		}
	}

	errHandler := func(err error) {
		log.Println(err)
	}

	// Start WebSocket for 1m candlestick data
	doneC, _, err := binance.WsKlineServe(symbol, "5m", wsKlineHandler, errHandler)
	if err != nil {
		log.Fatal(err)
	}

	<-doneC
}
