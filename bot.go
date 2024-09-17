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
	symbol         = "BTCUSDT"
	quantity       = 0.00130 // 500 USDT
	rsiPeriod      = 14
	rsiOverbought  = 69.0
	rsiOversold    = 30.0
	checkInterval  = 5 * time.Minute // Intervalo de 10 minutos
)

// Carga las variables de entorno desde el archivo .env
func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error al cargar el archivo .env")
	}
}

// Inicializa el cliente de Binance con las claves API
func initBinanceClient() {
	apiKey := os.Getenv("BINANCE_API_KEY")
	apiSecret := os.Getenv("BINANCE_API_SECRET")
	client = binance.NewClient(apiKey, apiSecret)
}

// Obtiene los precios de cierre de las últimas velas de 10 minutos
// Obtenemos las velas históricas de Binance para el símbolo BTC/USDT
func getHistoricalData(symbol string, limit int) ([]float64, error) {
    klines, err := client.NewKlinesService().Symbol(symbol).
        Interval("5m"). // Usamos intervalos de 5 minutos
        Limit(100).   // Obtenemos el número de velas solicitado
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

// Calcula el RSI basado en los precios de cierre utilizando SMA
func calculateRSIWithSMA(closes []float64, period int) float64 {
	var gains, losses []float64

	// Inicializamos las ganancias y pérdidas
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

	// Calcular las ganancias y pérdidas promedio usando SMA
	averageGain := sum(gains[:period]) / float64(period)
	averageLoss := sum(losses[:period]) / float64(period)

	// Ahora aplicamos la SMA en cada paso a partir del período 15 en adelante
	for i := period; i < len(gains); i++ {
		averageGain = (averageGain*(float64(period-1)) + gains[i]) / float64(period)
		averageLoss = (averageLoss*(float64(period-1)) + losses[i]) / float64(period)
	}

	// Si no hay pérdidas, el RSI es 100 (sobrecompra extrema)
	if averageLoss == 0 {
		return 100
	}

	// Cálculo del RSI
	rs := averageGain / averageLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// Función auxiliar para sumar un slice de float64
func sum(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}

// Obtiene el balance disponible de un activo
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
	return 0, fmt.Errorf("asset %s no encontrado", asset)
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
	return 0, fmt.Errorf("LOT_SIZE no encontrado para %s", symbol)
}

func trade(symbol string, side binance.SideType) {
	var balance float64
	var err error

	if side == binance.SideTypeSell {
		// Obtener el balance de BTC
		balance, err = getBalance("BTC")
		if err != nil {
			log.Println("Error al obtener el balance de BTC:", err)
			return
		}

		// Obtener el tamaño mínimo de lote para el par BTCUSDT
		stepSize, err := getLotSize(symbol)
		if err != nil {
			log.Println("Error al obtener el tamaño mínimo de lote:", err)
			return
		}

		// Redondear el balance al tamaño permitido por LOT_SIZE
		balance = roundToStepSize(balance, stepSize)

		// Verificar si el balance es mayor que cero después de redondear
		if balance <= 0 {
			log.Println("Balance insuficiente para realizar la operación.")
			return
		}
	} else {
		// Si es compra, usamos la cantidad fija definida
		balance = quantity
	}

	// Realizar la operación
	order, err := client.NewCreateOrderService().Symbol(symbol).
		Side(side).
		Type(binance.OrderTypeMarket).
		Quantity(fmt.Sprintf("%.6f", balance)).
		Do(context.Background())
	if err != nil {
		log.Println("Error al realizar la operación:", err)
		return
	}

	log.Printf("Operación ejecutada: %s %s %f", side, symbol, balance)
	log.Println("Detalles de la orden:", order)
}

// Verifica el RSI y ejecuta órdenes de compra o venta según el valor del RSI
func checkRSIAndTrade() {
	closes, err := getHistoricalData(symbol, rsiPeriod+1)
	if err != nil {
		log.Println("Error al obtener los datos históricos:", err)
		return
	}

	rsi := calculateRSIWithSMA(closes, rsiPeriod)
	log.Printf("RSI actual: %.2f", rsi)

	if rsi <= rsiOversold {
		log.Println("RSI por debajo de 30. Ejecutando compra.")
		trade(symbol,  binance.SideTypeBuy)
	} else if rsi >= rsiOverbought {
		log.Println("RSI por encima de 69. Ejecutando venta.")
		trade(symbol, binance.SideTypeSell)
	}
	closes = nil
}

func main() {
	// Cargar variables de entorno y configurar cliente de Binance

	loadEnv()
	initBinanceClient()

	// Ejecutar el chequeo del RSI cada 10 minutos

	
	for {
		checkRSIAndTrade()

		// Esperar 5 minutos antes de volver a ejecutar el chequeo
		time.Sleep(checkInterval)
	}
	
}
