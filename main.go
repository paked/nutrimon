package main

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
	"github.com/paked/gerrycode/communicator"
)

var (
	db *sql.DB
)

type User struct {
	ID       int64
	Username string
	Password string
}

func main() {
	var err error
	db, err = sql.Open("sqlite3", "database.db")
	if err != nil {
		log.Println("Could not open DB")
		return
	}

	r := mux.NewRouter()

	r.HandleFunc("/user/register", RegisterUserHandler).Methods("POST")

	http.Handle("/", r)

	log.Fatal(http.ListenAndServe("localhost:8080", nil))
}

func RegisterUserHandler(w http.ResponseWriter, r *http.Request) {
	c := communicator.New(w)
	u := User{}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		c.Fail("Incorrect user input")
		return
	}

	u, err := GetUser("username", username)
	if err == nil {
		c.Fail("That user already exists")
		return
	}

	u = User{
		Username: username,
		Password: password,
	}

	res, err := db.Exec("INSERT into users (username, password) VALUES (?, ?)", username, password)
	if err != nil {
		c.Fail("Could not insert user")
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		c.Fail("Could not get insert ID")
		return
	}

	u.ID = id

	c.OKWithData("Here is your user", u)
}

func GetUser(key, value string) (User, error) {
	u := User{}

	row := db.QueryRow("SELECT id, username, password, FROM users WHERE "+key+" = ?", value)
	err := row.Scan(&u.ID, &u.Username, &u.Password)
	if err != nil {
		return u, err
	}

	return u, nil
}
