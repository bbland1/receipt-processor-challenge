package main

import (
	"regexp"
	"time"

	"github.com/go-playground/validator/v10"
)

const (
	Description      = `^[\w\s&-]+$`
	ShortDescription = `^[\w\s\-]+$`
	Price            = `^\d+\.\d{2}$`

	DateFormat = "2006-01-02"
	TimeFormat = "15:04"
)

type User struct {
	ID string `json:"id"`
	Receipts []string `json:"receipts"`
}

type Merchant struct {
	Name string `json:"name"`
}

type IdResponse struct {
	ID string `json:"id"`
}

type ProcessedResponse struct {
	ID IdResponse
	Receipt ProcessedReceipt
}

type PointsResponse struct {
	Points int64 `json:"points"`
}

type ProcessedReceipt struct {
	ID      string `json:"id" validate:"required,uuid"`
	Receipt ReceiptPayload
	Points  int64 `json:"points" validate:"required"`
	SubmissionDate time.Time
	MerchantID string
	UserID string
}

type ReceiptPayload struct {
	Retailer     string `json:"retailer" validate:"required,retailerValidation"`
	PurchaseDate string `json:"purchaseDate" validate:"required,dateValidation"`
	PurchaseTime string `json:"purchaseTime" validate:"required,timeValidation"`
	Items        []Item `json:"items" validate:"required,min=1,dive"`
	Total        string `json:"total" validate:"required,priceValidation"`
}

type Item struct {
	ShortDescription string `json:"shortDescription" validate:"required,shortDescriptionValidation"`
	Price            string `json:"price" validate:"required,priceValidation"`
}

func shortDescriptionValidation(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	regEx := regexp.MustCompile(ShortDescription)
	return regEx.MatchString(value)
}

func retailerValidation(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	regEx := regexp.MustCompile(Description)
	return regEx.MatchString(value)
}

func priceValidation(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	regEx := regexp.MustCompile(Price)
	return regEx.MatchString(value)
}

func dateValidation(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	_, err := time.Parse(DateFormat, value)
	return err == nil
}

func timeValidation(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	_, err := time.Parse(TimeFormat, value)
	return err == nil
}
