package models

import (
	"time"
)

type Project struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	ApiKey    string    `json:"api_key"`
	SmtpUser  string    `json:"smtp_user"`
	SmtpPass  string    `json:"smtp_pass"`
	CreatedAt time.Time `json:"created_at"`
}

type Email struct {
	ID        int       `json:"id"`
	ProjectID int       `json:"project_id"`
	From      string    `json:"from"`
	To        []string  `json:"to"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Raw       string    `json:"raw"`
	Timestamp time.Time `json:"timestamp"`
}
