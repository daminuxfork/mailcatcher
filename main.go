// main.go
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mailtrap/internal/db"
	"mailtrap/internal/models"
	"net"
	"strings"
	"time"
)

var database *db.Database

func init() {
	var err error
	database, err = db.InitDB("mailcatcher.db")
	if err != nil {
		log.Fatal(err)
	}
}

func decodeBase64(encoded string) string {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		log.Printf("Erreur de décodage Base64: %v\n", err)
		return ""
	}
	return string(decoded)
}

// Nouvelle fonction pour gérer AUTH PLAIN
func handleAuthPlain(authData string) (username, password string) {
	data := decodeBase64(authData)
	parts := bytes.Split([]byte(data), []byte{0})
	if len(parts) == 3 {
		return string(parts[1]), string(parts[2])
	}
	return "", ""
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	conn.Write([]byte("220 smtp.localhost SMTP Service Ready\n"))

	reader := bufio.NewReader(conn)
	var currentProject *models.Project
	var from string
	var to []string
	var data strings.Builder
	var rawData strings.Builder
	var subject string
	var authenticated bool

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("Erreur de lecture: %v", err)
			}
			break
		}
		line = strings.TrimSpace(line)
		log.Printf("Commande reçue: %s", line)

		switch {
		case strings.HasPrefix(line, "HELO") || strings.HasPrefix(line, "EHLO"):
			conn.Write([]byte("250-smtp.localhost\n"))
			conn.Write([]byte("250-AUTH LOGIN PLAIN\n"))
			conn.Write([]byte("250 OK\n"))

		case strings.HasPrefix(line, "AUTH PLAIN"):
			parts := strings.Fields(line)
			var username, password string

			if len(parts) == 3 {
				// AUTH PLAIN pendant la commande
				username, password = handleAuthPlain(parts[2])
			} else {
				// AUTH PLAIN en deux étapes
				conn.Write([]byte("334 \n"))
				authData, _ := reader.ReadString('\n')
				username, password = handleAuthPlain(authData)
			}

			project, err := database.GetProjectBySmtpCredentials(username, password)
			if err == nil {
				authenticated = true
				currentProject = project
				conn.Write([]byte("235 2.7.0 Authentication successful\n"))
			} else {
				log.Printf("Échec d'authentification pour l'utilisateur: %s", username)
				conn.Write([]byte("535 5.7.8 Authentication failed\n"))
			}

		case strings.HasPrefix(line, "AUTH LOGIN"):
			conn.Write([]byte("334 VXNlcm5hbWU6\n")) // "Username:" en Base64
			usernameEnc, _ := reader.ReadString('\n')
			username := strings.TrimSpace(decodeBase64(usernameEnc))

			conn.Write([]byte("334 UGFzc3dvcmQ6\n")) // "Password:" en Base64
			passwordEnc, _ := reader.ReadString('\n')
			password := strings.TrimSpace(decodeBase64(passwordEnc))

			project, err := database.GetProjectBySmtpCredentials(username, password)
			if err == nil {
				authenticated = true
				currentProject = project
				conn.Write([]byte("235 2.7.0 Authentication successful\n"))
			} else {
				log.Printf("Échec d'authentification pour l'utilisateur: %s", username)
				conn.Write([]byte("535 5.7.8 Authentication failed\n"))
			}

		case strings.HasPrefix(line, "MAIL FROM:"):
			if !authenticated {
				conn.Write([]byte("530 5.7.0 Authentication required\n"))
				continue
			}
			from = strings.Trim(strings.TrimPrefix(line, "MAIL FROM:"), "<>")
			conn.Write([]byte("250 2.1.0 Ok\n"))

		case strings.HasPrefix(line, "RCPT TO:"):
			if !authenticated {
				conn.Write([]byte("530 5.7.0 Authentication required\n"))
				continue
			}
			recipient := strings.Trim(strings.TrimPrefix(line, "RCPT TO:"), "<>")
			to = append(to, recipient)
			conn.Write([]byte("250 2.1.5 Ok\n"))

		case line == "DATA":
			if !authenticated {
				conn.Write([]byte("530 5.7.0 Authentication required\n"))
				continue
			}
			conn.Write([]byte("354 End data with <CR><LF>.<CR><LF>\n"))

			data.Reset()
			rawData.Reset()
			fmt.Println("\n=== DÉBUT DES DONNÉES BRUTES ===")

			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					log.Printf("Erreur de lecture des données: %v", err)
					break
				}

				fmt.Print(dataLine)
				rawData.WriteString(dataLine)

				if strings.TrimSpace(dataLine) == "." {
					break
				}

				if strings.HasPrefix(strings.ToLower(dataLine), "subject:") && subject == "" {
					subject = strings.TrimSpace(strings.TrimPrefix(dataLine, "Subject:"))
				}
				data.WriteString(dataLine)
			}

			fmt.Println("=== FIN DES DONNÉES BRUTES ===\n")

			conn.Write([]byte("250 2.0.0 Ok: queued\n"))

			if authenticated && from != "" && len(to) > 0 {
				log.Printf("Préparation de l'enregistrement de l'email :")
				log.Printf("- Project ID : %d", currentProject.ID)
				log.Printf("- From : %s", from)
				log.Printf("- To : %v", to)
				log.Printf("- Subject : %s", subject)
				log.Printf("- Body length : %d", len(data.String()))
				log.Printf("- Raw length : %d", len(rawData.String()))

				email := &models.Email{
					ProjectID: currentProject.ID,
					From:      from,
					To:        to,
					Subject:   subject,
					Body:      data.String(),
					Raw:       rawData.String(),
					Timestamp: time.Now(),
				}

				if err := database.SaveEmail(email); err != nil {
					log.Printf("Erreur lors de la sauvegarde de l'email: %v", err)
				} else {
					log.Printf("Email enregistré avec succès")
				}
			} else {
				log.Printf("Email non enregistré car :")
				log.Printf("- Authenticated : %v", authenticated)
				log.Printf("- From non vide : %v", from != "")
				log.Printf("- To non vide : %v", len(to) > 0)
			}

		case line == "QUIT":
			conn.Write([]byte("221 2.0.0 Bye\n"))
			return

		default:
			conn.Write([]byte("500 5.5.1 Unknown command\n"))
		}
	}
}

func main() {
	ln, err := net.Listen("tcp", ":2525")
	if err != nil {
		log.Fatalf("Impossible de démarrer le serveur: %v\n", err)
	}
	defer ln.Close()

	log.Printf("Démarrage du serveur SMTP sur le port 2525\n")
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Erreur d'acceptation de connexion: %v\n", err)
			continue
		}
		go handleConnection(conn)
	}
}
