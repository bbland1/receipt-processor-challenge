package main

import (
	"encoding/json"
	"errors"
	"fmt"
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
var userStore = make(map[string][]string)

var (
	errInvalidJSON       = errors.New("invalid JSON format")
	errReceiptValidation = errors.New("validation issue")
	errGetReceiptById    = errors.New("no receipt found with the ID")
	errDateTimePointParse = errors.New("error in parsing purchase datetime")
	errTotalAsFloat = errors.New("total couldn't be parse as a float")
)

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
	token := r.Header.Get("X-Authorization")
	if err := json.NewDecoder(r.Body).Decode(&receipt); err != nil {
		return errInvalidJSON
	}

	if err := validate.Struct(receipt); err != nil {

		return fmt.Errorf("%w: %v", errReceiptValidation, strings.Split(err.Error(), ":")[2])
	}

	var processedReceipt *ProcessedReceipt
	newReceiptId := uuid.New().String()

	processedReceipt = &ProcessedReceipt{
		ID:      newReceiptId,
		Receipt: *receipt,
		Points:  0,
		UserID: token,
		MerchantID: receipt.Retailer,
		SubmissionDate: time.Now(),
	}

	points, err := processReceiptPoints(&processedReceipt.Receipt, token)
	if err != nil {
		return err
	}

	processedReceipt.Points = points

	userStore[token] = append(userStore[token], newReceiptId)

	receiptStore[newReceiptId] = processedReceipt

	response := IdResponse{
		ID: processedReceipt.ID,
	}

	return WriteJson(w, http.StatusOK, response)
}

// handleGetPointsById takes a receipt id value and returns the determined points for that receipt.
func (s *ApiServer) handleGetPointsById(w http.ResponseWriter, r *http.Request) error {
	vars := mux.Vars(r)
	id := vars["id"]

	receipt, ok := receiptStore[id]

	if !ok {
		return fmt.Errorf("%w: %s", errGetReceiptById, id)
	}

	response := PointsResponse{
		Points: receipt.Points,
	}

	return WriteJson(w, http.StatusOK, response)
}

// WriteJson sends JSON response to client with status code.
func WriteJson(w http.ResponseWriter, statusCode int, value any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(value)
}

type ApiHandlerFunc func(http.ResponseWriter, *http.Request) error

type ApiError struct {
	Error string `json:"error"`
}

// errorHandleToHandleFunc is a wrapper to handle the error returned by the route logics and make sure it is a HandlerFunc that is required by the mux router and will send the error that happen in the route.
func errorHandleToHandleFunc(fn ApiHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			if errors.Is(err, errInvalidJSON) || errors.Is(err, errReceiptValidation) {
				WriteJson(w, http.StatusBadRequest, ApiError{Error: err.Error()})
				return
			}

			if errors.Is(err, errGetReceiptById) {
				WriteJson(w, http.StatusNotFound, ApiError{Error: err.Error()})
				return
			}

			if errors.Is(err, errDateTimePointParse) || errors.Is(err, errTotalAsFloat){
				WriteJson(w, http.StatusBadRequest, ApiError{Error: err.Error()})
				return
			}

			WriteJson(w, http.StatusInternalServerError, ApiError{Error: err.Error()})

		}
	}
}

// processReceiptPoints takes a receipt and processes it to return the point value based on the establish value logic.
func processReceiptPoints(receipt *ReceiptPayload, userID string) (int64, error) {
	/*
		current point logic:
		- 1 point for every alphanumeric character in the retailer name.
		- 50 points if the total is a round dollar amount with no cents.
		- 25 points if the total is a multiple of 0.25.
		- 5 points for every two items on the receipt.
		- If the trimmed length of the item description is a multiple of 3,
		-  multiply the price by 0.2 and round up to the nearest integer. The result is the number of points earned.
		- 6 points if the day in the purchase date is odd.
		- 10 points if the time of purchase is after 2:00pm and before 4:00pm.
		- we are running a promotions for the first 3 receipts scanned for the user they will get 300 points for each receipt
	*/
	pointValue := 0

	totalAsFloat, err := strconv.ParseFloat(receipt.Total, 64)
	if err != nil {
		return 0, errTotalAsFloat
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

	purchaseDateTimeParse, err := time.Parse(fmt.Sprintf("%s %s", DateFormat, TimeFormat), fmt.Sprintf("%s %s", receipt.PurchaseDate, receipt.PurchaseTime))
	if err != nil {
		return 0, errDateTimePointParse
	}

	if purchaseDateTimeParse.Day()%2 == 1 {
		pointValue += 6
	}

	after2pm := time.Date(purchaseDateTimeParse.Year(), purchaseDateTimeParse.Month(), purchaseDateTimeParse.Day(), 14, 0, 0, 0, time.UTC)
	before4pm := time.Date(purchaseDateTimeParse.Year(), purchaseDateTimeParse.Month(), purchaseDateTimeParse.Day(), 16, 0, 0, 0, time.UTC)

	if purchaseDateTimeParse.After(after2pm) && purchaseDateTimeParse.Before(before4pm) {
		pointValue += 10
	}

	// when a user has 
	if len(userStore[userID]) < 3 {
		pointValue += 300
	}

	return int64(pointValue), nil
}
