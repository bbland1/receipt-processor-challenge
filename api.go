package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

var validate *validator.Validate
var receiptStore = make(map[string]*ProcessedReceipt)

func init() {
	validate = validator.New()

	validate.RegisterValidation("retailerValidation", retailerValidation)
	validate.RegisterValidation("shortDescriptionValidation", shortDescriptionValidation)
	validate.RegisterValidation("priceValidation", priceValidation)
	validate.RegisterValidation("dateValidation", dateValidation)
	validate.RegisterValidation("timeValidation", timeValidation)
}

type ApiServer struct {
	listenAddr string
}

// NewApiServer creates and initializes a new instance of ApiServer with the passed port/address info.
func NewApiServer(address string) *ApiServer {
	return &ApiServer{
		listenAddr: address,
	}
}

// Run is called to start the server on the passed port.
func (s *ApiServer) Run() {
	router := mux.NewRouter()

	router.HandleFunc("/receipts/process", errorHandleToHandleFunc(s.handleProcessReceipts)).Methods("POST")
	router.HandleFunc("/receipts/{id}/points", errorHandleToHandleFunc(s.handleGetPointsById)).Methods("GET")

	log.Println("server running on port:", s.listenAddr)
	http.ListenAndServe(s.listenAddr, router)
}

// handleProcessReceipts takes a JSON payload of a receipt to determine the points values of the receipt add to the info and return an id for the successfully processed receipt.
func (s *ApiServer) handleProcessReceipts(w http.ResponseWriter, r *http.Request) error {
	var receipt *ReceiptPayload
	if err := json.NewDecoder(r.Body).Decode(&receipt); err != nil {
		return err
	}

	if err := validate.Struct(receipt); err != nil {

		return err
	}

	var processedReceipt *ProcessedReceipt
	newReceiptId := uuid.New().String()

	points, err := processReceiptPoints(receipt)
	if err != nil {
		return err
	}

	processedReceipt = &ProcessedReceipt{
		ID:      newReceiptId,
		Receipt: *receipt,
		Points:  points,
	}

	receiptStore[newReceiptId] = processedReceipt

	return WriteJson(w, http.StatusOK, processedReceipt.Points)
}

// handleGetPointsById takes a receipt id value and returns the determined points for that receipt.
func (s *ApiServer) handleGetPointsById(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func WriteJson(w http.ResponseWriter, statusCode int, value any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(value)
}

type ApiHandlerFunc func(http.ResponseWriter, *http.Request) error

type ApiError struct {
	Error   string `json:"error"`
	Message string `json:"msg"`
}

func errorHandleToHandleFunc(fn ApiHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			WriteJson(w, http.StatusBadRequest, ApiError{Error: "The receipt is invalid", Message: err.Error()})
		}
	}
}

func processReceiptPoints(receipt *ReceiptPayload) (int64, error) {
	pointValue := 0

	totalAsFloat, err := strconv.ParseFloat(receipt.Total, 64)
	if err != nil {
		return 0, err
	}

	for _, char := range receipt.Retailer {
		if unicode.IsLetter(char) || unicode.IsNumber(char) {
			pointValue += 1
		}
	}

	if math.Mod(totalAsFloat, 1) == 0 {
		pointValue += 50
	}

	if math.Mod(totalAsFloat, 0.25) == 0 {
		pointValue += 25
	}

	if len(receipt.Items) >= 2 {
		pointValue += (len(receipt.Items) / 2) * 5
	}

	for _, item := range receipt.Items {
		if len(strings.TrimSpace(item.ShortDescription))%3 == 0 {
			priceAsFloat, err := strconv.ParseFloat(item.Price, 64)
			if err != nil {
				return 0, err
			}

			pointValue += int(math.Ceil(priceAsFloat * 0.2))
		}
	}

	purchaseDateParse, err := time.Parse(DateFormat, receipt.PurchaseDate)
	if err != nil {
		return 0, err
	}

	if purchaseDateParse.Day() % 2 == 1 {
		pointValue += 6
	}

	purchaseTimeParse, err := time.Parse(TimeFormat, receipt.PurchaseTime)
	if err != nil {
		return 0, err
	}

	after2pm := time.Date(0,0,0, 14, 0,0,0, time.UTC)
	before4pm := time.Date(0,0,0, 16, 0,0,0, time.UTC)

	if purchaseTimeParse.After(after2pm) && purchaseTimeParse.Before(before4pm) {
		pointValue += 10
	}

	return int64(pointValue), nil
}
