package main

import (
	"regexp"
	"time"

	"github.com/go-playground/validator/v10"
)

const (
	Description      = `^[\\w\\s&-]+$`
	ShortDescription = `^[\\w\\s\\-]+$`
	Price            = `^\\d+\\.\\d{2}$`

	DateFormat = "2006-01-02"
	TimeFormat = "15:04"
)

type Receipt struct {
	Retailer     string `json:"description" validate:"required,descriptionValidation"`
	PurchaseDate string `json:"purchaseDate" validate:"required"`
	PurchaseTime string `json:"purchaseTime" validate:"required"`
	Items        []*Item `json:"items" validate:"required, gt=0,dive"`
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

func descriptionValidation(fl validator.FieldLevel) bool {
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
