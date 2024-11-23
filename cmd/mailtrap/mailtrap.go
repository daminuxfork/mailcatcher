// cmd/mailtrap/main.go
package main

import (
	"bufio"
	"fmt"
	"log"
	"mailtrap/internal/db"
	"os"
	"strconv"
	"strings"
)

var database *db.Database
var reader *bufio.Reader

func init() {
	var err error
	database, err = db.InitDB("mailcatcher.db")
	if err != nil {
		log.Fatal(err)
	}
	reader = bufio.NewReader(os.Stdin)
}

func readInput(prompt string) string {
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func showMenu() {
	fmt.Println("\nMailtrap CLI - Menu Principal")
	fmt.Println("============================")
	fmt.Println("1. Créer un nouveau projet")
	fmt.Println("2. Lister les projets")
	fmt.Println("3. Lister les emails")
	fmt.Println("4. Vérifier la base de données")
	fmt.Println("5. Réinitialiser la base de données")
	fmt.Println("0. Quitter")
	fmt.Println("============================")
}

func createProject() {
	fmt.Println("\nCréation d'un nouveau projet")
	fmt.Println("=========================")

	name := readInput("Nom du projet : ")
	if name == "" {
		fmt.Println("Le nom du projet ne peut pas être vide")
		return
	}

	user := readInput("Nom d'utilisateur SMTP : ")
	if user == "" {
		fmt.Println("Le nom d'utilisateur ne peut pas être vide")
		return
	}

	pass := readInput("Mot de passe SMTP : ")
	if pass == "" {
		fmt.Println("Le mot de passe ne peut pas être vide")
		return
	}

	project, err := database.CreateProject(name, user, pass)
	if err != nil {
		fmt.Printf("Erreur lors de la création du projet : %v\n", err)
		return
	}

	fmt.Println("\nProjet créé avec succès !")
	fmt.Println("---------------------------")
	fmt.Printf("ID: %d\n", project.ID)
	fmt.Printf("Nom: %s\n", project.Name)
	fmt.Printf("API Key: %s\n", project.ApiKey)
	fmt.Printf("SMTP Username: %s\n", project.SmtpUser)
	fmt.Printf("SMTP Password: %s\n\n", project.SmtpPass)

	fmt.Println("Pour tester avec swaks :")
	fmt.Printf("swaks --server localhost:2525 --auth-user %s --auth-password %s --to recipient@example.com --from sender@example.com --header \"Subject: Test\" --body \"Test\"\n",
		project.SmtpUser, project.SmtpPass)
}

func listProjects() {
	projects, err := database.ListProjects()
	if err != nil {
		fmt.Printf("Erreur lors de la récupération des projets : %v\n", err)
		return
	}

	if len(projects) == 0 {
		fmt.Println("\nAucun projet trouvé")
		return
	}

	fmt.Println("\nListe des projets :")
	fmt.Println("==================")
	for _, project := range projects {
		fmt.Printf("\nID: %d\n", project.ID)
		fmt.Printf("Nom: %s\n", project.Name)
		fmt.Printf("API Key: %s\n", project.ApiKey)
		fmt.Printf("SMTP Username: %s\n", project.SmtpUser)
		fmt.Printf("SMTP Password: %s\n", project.SmtpPass)
		fmt.Printf("Créé le: %s\n", project.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Println("------------------")
	}
}

func listEmails() {
	fmt.Println("\nLister les emails")
	fmt.Println("1. Tous les emails")
	fmt.Println("2. Emails d'un projet spécifique")

	choice := readInput("Votre choix : ")
	var projectID int = 0

	if choice == "2" {
		idStr := readInput("ID du projet : ")
		var err error
		projectID, err = strconv.Atoi(idStr)
		if err != nil {
			fmt.Println("ID de projet invalide")
			return
		}
	}

	rows, err := database.Query(`
        SELECT e.id, p.name, e.from_addr, e.to_addr, e.subject, e.timestamp, e.raw
        FROM emails e
        JOIN projects p ON e.project_id = p.id
        WHERE (? = 0 OR e.project_id = ?)
        ORDER BY e.timestamp DESC
    `, projectID, projectID)

	if err != nil {
		fmt.Printf("Erreur lors de la récupération des emails : %v\n", err)
		return
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		count++
		var emailID int
		var projectName, from, to, subject, raw string
		var timestamp string

		err := rows.Scan(&emailID, &projectName, &from, &to, &subject, &timestamp, &raw)
		if err != nil {
			fmt.Printf("Erreur lors de la lecture d'un email : %v\n", err)
			continue
		}

		fmt.Println("\n=================================")
		fmt.Printf("Email ID: %d\n", emailID)
		fmt.Printf("Projet: %s\n", projectName)
		fmt.Printf("Date: %s\n", timestamp)
		fmt.Printf("De: %s\n", from)
		fmt.Printf("À: %s\n", to)
		fmt.Printf("Sujet: %s\n", subject)
		fmt.Println("\nContenu brut:")
		fmt.Println("---------------------------------")
		fmt.Println(raw)
		fmt.Println("=================================")
	}

	if count == 0 {
		fmt.Println("\nAucun email trouvé")
	} else {
		fmt.Printf("\nNombre total d'emails : %d\n", count)
	}
}

func checkDatabase() {
	fmt.Println("\nVérification de la base de données")
	fmt.Println("================================")

	// Vérifier la structure de la table emails
	rows, err := database.Query(`
        SELECT sql FROM sqlite_master 
        WHERE type='table' AND name IN ('emails', 'projects')
    `)
	if err != nil {
		fmt.Printf("Erreur : %v\n", err)
		return
	}
	defer rows.Close()

	fmt.Println("\nStructure des tables :")
	for rows.Next() {
		var sql string
		rows.Scan(&sql)
		fmt.Println(sql)
	}

	// Compter les lignes
	var emailCount, projectCount int
	database.QueryRow("SELECT COUNT(*) FROM emails").Scan(&emailCount)
	database.QueryRow("SELECT COUNT(*) FROM projects").Scan(&projectCount)

	fmt.Printf("\nStatistiques :\n")
	fmt.Printf("- Nombre de projets : %d\n", projectCount)
	fmt.Printf("- Nombre d'emails : %d\n", emailCount)
}

func resetDatabase() {
	confirm := readInput("\nATTENTION: Cette action va supprimer toutes les données. Confirmer ? (oui/non) : ")
	if strings.ToLower(confirm) != "oui" {
		fmt.Println("Opération annulée")
		return
	}

	os.Remove("mailcatcher.db")
	database.Close()

	var err error
	database, err = db.InitDB("mailcatcher.db")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Base de données réinitialisée avec succès")
}

func main() {
	defer database.Close()

	for {
		showMenu()
		choice := readInput("Votre choix : ")

		switch choice {
		case "1":
			createProject()
		case "2":
			listProjects()
		case "3":
			listEmails()
		case "4":
			checkDatabase()
		case "5":
			resetDatabase()
		case "0":
			fmt.Println("Au revoir !")
			return
		default:
			fmt.Println("Choix invalide")
		}
	}
}
