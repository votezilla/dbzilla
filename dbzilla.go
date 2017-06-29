package main

import (
	"database/sql"
	"fmt"
	"html/template"
//	"io/ioutil"
	"log"
	"net/http"
//	"regexp"
//  "reflect"
	"time"

	_ "github.com/lib/pq"
)

var database  string
var db        *sql.DB
var table     string
var templates *template.Template
var query     string
var err       error // Just so we don't have to continually redefine it, for catching error messages.

type Row       []string
type Tables    []string
type Columns   Row
type Databases Row
type Data      []Row

var (
	dbUser = "________"
	dbPassword = "_________"  // TODO: pass in pw via command-line arg
)

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func connectDb(database string) (*sql.DB, error) {
	return sql.Open(
		"postgres", 
		fmt.Sprintf(
			"user='%s' password='%s' dbname='%s' sslmode=disable", 
			dbUser, 
			dbPassword, 
			database))
}

func queryDb(query string) (Columns, Data, error) {
	var columns Columns
	var rows    *sql.Rows
	var data    Data
	
	rows, err = db.Query(query)
	if err != nil {
		return Columns{}, Data{}, err
	}
	defer rows.Close()

	columns, err = rows.Columns()
	if err != nil {
		return Columns{}, Data{}, err
	}

	count := len(columns)
	values := make([]interface{}, count)
	valuePtrs := make([]interface{}, count)

	for rows.Next() {
		for i, _ := range columns {
			valuePtrs[i] = &values[i]
		}

		rows.Scan(valuePtrs...)

		rowData := make(Row, count)
		for i, _ := range columns {
			val := values[i]

			b, ok := val.([]byte)

			if ok {
				rowData[i] = string(b)
			} else {
				rowData[i] = fmt.Sprintf("%v", val)
			}
		}

		data = append(data, rowData);
    }
    
	return columns, data, err
}

func listDbs() Databases {
	var databases Databases
	
	_, data, err := queryDb("SELECT datname FROM pg_database")
	check(err)
	
	for _, row := range data {
		if row[0] != "template0" && row[0] != "template1" {
			databases = append(databases, row[0])
		}
	}
	
	return databases
}

func listTables() Tables {
	var tables Tables

	// List tables in this database
	if database == "postgres" { // Where postgres tables are stored:
		_, data, err := queryDb("SELECT tablename FROM pg_catalog.pg_tables")

		if err != nil {
			return nil
		}

		for _, row := range data {
			tables = append(tables, row[0])
		}

		return tables	
	} else {                    // Where user-created tables are stored:
		// Hack: have to hardcode all table schemas we use here.  It's difficult to ignore system tables otherwise.
		_, data, err := queryDb(
			`SELECT table_schema, table_name FROM information_schema.tables 
				WHERE (table_schema='public' OR table_schema='votezilla') AND table_type='BASE TABLE'`)

		if err != nil {
			return nil
		}

		for _, row := range data {
			schema_name := row[0]
			table_name := row[1]
			
			if schema_name == "public" {
				tables = append(tables, table_name)
			} else {
				tables = append(tables, schema_name + "." + table_name)
			}
		}

		return tables	
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	if database == "" {
		cookie, _ := r.Cookie("db")
		fmt.Printf("got cookie: %v", cookie)
		
		database = cookie.Value
			
		fmt.Printf("got database from cookie: %s", database)
	}
	
	// By default, and also it creases errors if we access "template0" or "template1",
	// so use database "postgres" by default in these cases.
	if database == "" || database == "template0"{
		database = "postgres"
	}
	log.Printf("handler database:", database)

	db, err = connectDb(database)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	
	databases := listDbs()

	tables := listTables()
	
	w.Header().Set("Content-Type", "text/html")
	
	//DESCRIBE TABLE in Postgres
	var columns Columns
	var data    Data
	
	fmt.Println("query: ", query);
	if query != "" {
		columns, data, err = queryDb(query)
	}
	
	if err != nil {
		query = "" // Reset the query so we dont keep in a loop of bad queries.
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	templateArgs := struct {
		Title     string
		Body      []byte
		Columns   Columns
		Database  string
		Databases Databases
		Data      Data
		Query     string
		Tables    Tables
	}{
		"", 
		[]byte(""), 
		columns,
		database,
		databases,
		data,
		query,
		tables,
	}
	
	err := templates.ExecuteTemplate(w, "dbquery.html", templateArgs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func selectDbHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
    database = r.FormValue("db")
    
    fmt.Println("database", database)
    
    cookieExpiration := time.Now().Add(365 * 24 * time.Hour)
    cookie := http.Cookie{Name: "db", Value: database, Expires: cookieExpiration}
    http.SetCookie(w, &cookie)
    
    handler(w, r)
}

func describeTableHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
    table = r.FormValue("table")
    
    fmt.Println("table", table)

    //DESCRIBE TABLE in Postgres
    if table != "" {
		query = fmt.Sprintf(
			"SELECT column_name, data_type, character_maximum_length FROM INFORMATION_SCHEMA.COLUMNS WHERE table_name = '%s'",
			table)
	}

	handler(w, r)
}

func selectAllHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
    table := r.FormValue("table")
    fmt.Println("table", table)
    
    if table != "" {
	    query = fmt.Sprintf("SELECT * FROM %s", table)
	}

    handler(w, r)
}

func selectCountHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
    table := r.FormValue("table")
    fmt.Println("table", table)
    
    if table != "" {
    	query = fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	}

    handler(w, r)
}

func queryHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
    query = r.FormValue("query")
    
    if query != "" {
    	fmt.Println("query", query)
	}

    handler(w, r)
}

func init() {
	log.Printf("init")
	
	var err error
	templates, err = template.ParseFiles("templates/dbquery.html")
	check(err)
}

func main() {
	http.HandleFunc("/SelectDb",      selectDbHandler)
	http.HandleFunc("/DescribeTable", describeTableHandler)
	http.HandleFunc("/SelectAll",     selectAllHandler)
	http.HandleFunc("/SelectCount",   selectCountHandler)
	http.HandleFunc("/Query",         queryHandler)
	http.HandleFunc("/", handler)
	
	log.Printf("XXXXX About to listen on 9090. Go to https:127.0.0.1:9090/")
	err := http.ListenAndServe(":9090", nil)
	log.Fatal(err)
}
