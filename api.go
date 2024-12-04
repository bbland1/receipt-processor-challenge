package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

var validate *validator.Validate

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
	processedReceipt = &ProcessedReceipt{
		ID:      newReceiptId,
		Receipt: *receipt,
		Points: 0,
	}

	return WriteJson(w, http.StatusOK, processedReceipt.ID)
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
