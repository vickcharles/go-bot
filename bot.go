package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/joho/godotenv"
)

var client *binance.Client

const (
	symbol        = "BTCUSDT"
	quantity      = 0.00130
	rsiPeriod     = 14
	rsiOverbought = 69.0
	rsiOversold   = 30.0
	checkInterval = 5 * time.Minute
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

// Get the available balance of an asset
func getBalance(asset string) (float64, error) {
	account, err := client.NewGetAccountService().Do(context.Background())
	if err != nil {
		return 0, err
	}
	for _, balance := range account.Balances {
		if balance.Asset == asset {
			freeBalance, _ := strconv.ParseFloat(balance.Free, 64)
			return freeBalance, nil
		}
	}
	return 0, fmt.Errorf("asset %s not found", asset)
}

func roundToStepSize(quantity, stepSize float64) float64 {
	return math.Floor(quantity/stepSize) * stepSize
}

func getLotSize(symbol string) (float64, error) {
	exchangeInfo, err := client.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return 0, err
	}

	for _, s := range exchangeInfo.Symbols {
		if s.Symbol == symbol {
			for _, filter := range s.Filters {
				if filter["filterType"] == "LOT_SIZE" {
					stepSize, _ := strconv.ParseFloat(filter["stepSize"].(string), 64)
					return stepSize, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("LOT_SIZE not found for %s", symbol)
}

func trade(symbol string, side binance.SideType) {
	var balance float64
	var err error

	if side == binance.SideTypeSell {
		// Get BTC balance
		balance, err = getBalance("BTC")
		if err != nil {
			log.Println("Error getting BTC balance:", err)
			return
		}

		// Get the minimum lot size for BTCUSDT
		stepSize, err := getLotSize(symbol)
		if err != nil {
			log.Println("Error getting minimum lot size:", err)
			return
		}

		// Round the balance to the allowed LOT_SIZE
		balance = roundToStepSize(balance, stepSize)

		// Check if the balance is greater than zero after rounding
		if balance <= 0 {
			log.Println("Insufficient balance to execute the operation.")
			return
		}
	} else {
		// For buy orders, use the fixed quantity
		balance = quantity
	}

	// Execute the order
	order, err := client.NewCreateOrderService().Symbol(symbol).
		Side(side).
		Type(binance.OrderTypeMarket).
		Quantity(fmt.Sprintf("%.6f", balance)).
		Do(context.Background())
	if err != nil {
		log.Println("Error executing the operation:", err)
		return
	}

	log.Printf("Operation executed: %s %s %f", side, symbol, balance)
	log.Println("Order details:", order)
}

// Check RSI and execute buy or sell orders based on RSI value
func checkRSIAndTrade() {
	closes, err := getHistoricalData(symbol, rsiPeriod+1)
	if err != nil {
		log.Println("Error getting historical data:", err)
		return
	}

	rsi := calculateRSIWithSMA(closes, rsiPeriod)
	log.Printf("Current RSI: %.2f", rsi)

	if rsi <= rsiOversold {
		log.Println("RSI below 30. Executing buy.")
		trade(symbol, binance.SideTypeBuy)
	} else if rsi >= rsiOverbought {
		log.Println("RSI above 69. Executing sell.")
		trade(symbol, binance.SideTypeSell)
	}
	closes = nil
}

func main() {
	// Load environment variables and initialize Binance client
	loadEnv()
	initBinanceClient()

	// Run the RSI check at regular intervals
	for {
		checkRSIAndTrade()
		time.Sleep(checkInterval)
	}
}
