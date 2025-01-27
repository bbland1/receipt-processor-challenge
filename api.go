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
	"sync"
	"time"
	"unicode"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

var validate *validator.Validate
var receiptStore = sync.Map{}
var userStore = sync.Map{}

var (
	errInvalidJSON        = errors.New("invalid JSON format")
	errReceiptValidation  = errors.New("validation issue")
	errGetReceiptById     = errors.New("no receipt found with the ID")
	errDateTimePointParse = errors.New("error in parsing purchase datetime")
	errTotalAsFloat       = errors.New("total couldn't be parse as a float")
	errUserNotFound       = errors.New("user not found to add points")
	errLoadOrStoreUser    = errors.New("error loading or storing user first time")
	errDuplicateReceipt   = errors.New("this receipt has been uploaded before")
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
	var receipts []*ReceiptPayload
	var user User
	// todo: this is a quick way to get various users set would need to update for prod
	token := r.Header.Get("X-Authorization")
	userObj, _ := userStore.LoadOrStore(token, User{ID: token, Receipts: []string{}})

	if userVal, ok := userObj.(User); ok {
		user = userVal
	} else {
		return errLoadOrStoreUser
	}

	if err := json.NewDecoder(r.Body).Decode(&receipts); err != nil {
		return errInvalidJSON
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(receipts))
	resChan := make(chan IdResponse, len(receipts))
	receiptChan := make(chan ProcessedReceipt, len(receipts))

	for _, receipt := range receipts {
		wg.Add(1)
		go func(receipt *ReceiptPayload, user User, errCh chan<- error, resCh chan<- IdResponse, receiptChan chan<- ProcessedReceipt) {
			defer wg.Done()

			if err := validate.Struct(receipt); err != nil {
				errCh <- fmt.Errorf("%w: %v", errReceiptValidation, strings.Split(err.Error(), ":")[2])
				return
			}

			isDupe := false
			receiptStore.Range(func(key, value any) bool {
				storedReceipt, ok := value.(*ProcessedReceipt)
				if !ok {
					return true
				}

				if storedReceipt.Receipt.Retailer == receipt.Retailer && storedReceipt.Receipt.Total == receipt.Total && storedReceipt.Receipt.PurchaseDate == receipt.PurchaseDate && storedReceipt.Receipt.PurchaseTime == receipt.PurchaseTime {
					isDupe = true
					return false
				}

				return true
			})

			if isDupe {
				errCh <- errDuplicateReceipt
			}

			var processedReceipt *ProcessedReceipt
			newReceiptId := uuid.New().String()

			processedReceipt = &ProcessedReceipt{
				ID:             newReceiptId,
				Receipt:        *receipt,
				Points:         0,
				UserID:         user.ID,
				MerchantID:     receipt.Retailer,
				SubmissionDate: time.Now(),
			}

			points, err := processReceiptPoints(&processedReceipt.Receipt, user.ID)
			if err != nil {
				errCh <- err
				return
			}

			processedReceipt.Points = points

			receiptChan <- *processedReceipt

			response := IdResponse{
				ID: processedReceipt.ID,
			}

			resCh <- response
		}(receipt, user, errChan, resChan, receiptChan)
	}

	go func() {
		wg.Wait()
		close(errChan)
		close(resChan)
		close(receiptChan)

	}()

	var response []IdResponse

	fmt.Println(receiptStore)
	fmt.Println(userStore)
	for {
		select {
		case res, ok := <-resChan:
			if !ok {
				resChan = nil
			} else {
				response = append(response, res)
			}
		case err, ok := <-errChan:
			if ok && err != nil {
				return err
			}
			if !ok {
				errChan = nil
			}
		case receipt, ok := <-receiptChan:
			if !ok {
				receiptChan = nil
			} else {
				receiptStore.Store(receipt.ID, receipt)
				storedInfo, ok := userStore.Load(receipt.UserID)

				if ok {
					userInfo, ok := storedInfo.(User)
					if !ok {
						return errLoadOrStoreUser
					}

					userInfo.Receipts = append(userInfo.Receipts, receipt.ID)
					userStore.CompareAndSwap(userInfo.ID, userInfo.Receipts, userInfo.Receipts)
				}

				userStore.Store(receipt.UserID, []string{receipt.ID})

			}
		}
		if errChan == nil && resChan == nil {
			break
		}
	}

	return WriteJson(w, http.StatusOK, response)
}

// handleGetPointsById takes a receipt id value and returns the determined points for that receipt.
func (s *ApiServer) handleGetPointsById(w http.ResponseWriter, r *http.Request) error {
	vars := mux.Vars(r)
	id := vars["id"]

	receiptVal, ok := receiptStore.Load(id)

	if !ok {
		return fmt.Errorf("%w: %s", errGetReceiptById, id)
	}

	receipt := receiptVal.(ProcessedReceipt)

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

			if errors.Is(err, errDateTimePointParse) || errors.Is(err, errTotalAsFloat) {
				WriteJson(w, http.StatusBadRequest, ApiError{Error: err.Error()})
				return
			}

			if errors.Is(err, errUserNotFound) {
				WriteJson(w, http.StatusUnauthorized, ApiError{Error: err.Error()})
				return
			}

			// ? a different code is probably better late night brain blah -_-
			if errors.Is(err, errLoadOrStoreUser) {
				WriteJson(w, http.StatusNotModified, ApiError{Error: err.Error()})
				return
			}

			// ? a different code is probably better late night brain blah -_-
			if errors.Is(err, errDuplicateReceipt) {
				WriteJson(w, http.StatusConflict, ApiError{Error: err.Error()})
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

	userVal, ok := userStore.Load(userID)

	if !ok {
		return 0, errUserNotFound
	}

	user := userVal.(User)
	if len(user.Receipts) < 3 {
		pointValue += 300
	}

	return int64(pointValue), nil
}
