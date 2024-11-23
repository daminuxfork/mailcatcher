// internal/db/database.go
package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"log"
	"mailtrap/internal/models"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Database struct {
	*sql.DB
}

// generateApiKey génère une clé API aléatoire
func generateApiKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func InitDB(dataSourceName string) (*Database, error) {
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	database := &Database{db}
	database.createTables()
	return database, nil
}

func (db *Database) createTables() {
	// Création de la table projects seulement si elle n'existe pas
	projectsTable := `
    CREATE TABLE IF NOT EXISTS projects (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        api_key TEXT UNIQUE NOT NULL,
        smtp_user TEXT UNIQUE NOT NULL,
        smtp_pass TEXT NOT NULL,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );`

	// Création de la table emails seulement si elle n'existe pas
	emailsTable := `
    CREATE TABLE IF NOT EXISTS emails (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        project_id INTEGER,
        from_addr TEXT NOT NULL,
        to_addr TEXT NOT NULL,
        subject TEXT,
        body TEXT,
        raw TEXT,
        timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (project_id) REFERENCES projects(id)
    );`

	_, err := db.Exec(projectsTable)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(emailsTable)
	if err != nil {
		log.Fatal(err)
	}
}

func (db *Database) CreateProject(name, smtpUser, smtpPass string) (*models.Project, error) {
	apiKey, err := generateApiKey()
	if err != nil {
		return nil, err
	}

	stmt, err := db.Prepare(`
        INSERT INTO projects (name, api_key, smtp_user, smtp_pass, created_at)
        VALUES (?, ?, ?, ?, ?)
    `)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	now := time.Now()
	result, err := stmt.Exec(name, apiKey, smtpUser, smtpPass, now)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &models.Project{
		ID:        int(id),
		Name:      name,
		ApiKey:    apiKey,
		SmtpUser:  smtpUser,
		SmtpPass:  smtpPass,
		CreatedAt: now,
	}, nil
}

func (db *Database) GetProjectBySmtpCredentials(smtpUser, smtpPass string) (*models.Project, error) {
	project := &models.Project{}
	err := db.QueryRow(`
        SELECT id, name, api_key, smtp_user, smtp_pass, created_at
        FROM projects
        WHERE smtp_user = ? AND smtp_pass = ?`,
		smtpUser, smtpPass).Scan(
		&project.ID,
		&project.Name,
		&project.ApiKey,
		&project.SmtpUser,
		&project.SmtpPass,
		&project.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return project, nil
}

func (db *Database) SaveEmail(email *models.Email) error {
	toJSON, err := json.Marshal(email.To)
	if err != nil {
		return err
	}

	stmt, err := db.Prepare(`
        INSERT INTO emails (project_id, from_addr, to_addr, subject, body, raw, timestamp)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		email.ProjectID,
		email.From,
		string(toJSON),
		email.Subject,
		email.Body,
		email.Raw,
		email.Timestamp,
	)
	return err
}

func (db *Database) ListProjects() ([]models.Project, error) {
	rows, err := db.Query(`
        SELECT id, name, api_key, smtp_user, smtp_pass, created_at
        FROM projects
        ORDER BY id
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var project models.Project
		err := rows.Scan(
			&project.ID,
			&project.Name,
			&project.ApiKey,
			&project.SmtpUser,
			&project.SmtpPass,
			&project.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, nil
}

func (db *Database) ListEmails(projectID int) ([]models.Email, error) {
	rows, err := db.Query(`
        SELECT id, project_id, from_addr, to_addr, subject, body, raw, timestamp
        FROM emails
        WHERE project_id = ?
        ORDER BY timestamp DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []models.Email
	for rows.Next() {
		var email models.Email
		var toJSON string
		err := rows.Scan(
			&email.ID,
			&email.ProjectID,
			&email.From,
			&toJSON,
			&email.Subject,
			&email.Body,
			&email.Raw,
			&email.Timestamp,
		)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal([]byte(toJSON), &email.To)
		if err != nil {
			return nil, err
		}

		emails = append(emails, email)
	}
	return emails, nil
}
