package client

import (
	"log"

	"github.com/go-resty/resty/v2"
)

func logResponse(resp *resty.Response, err error) {
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	log.Printf("Response Status: %v", resp.Status())
	log.Printf("Response Body: %v", resp.String())
}
