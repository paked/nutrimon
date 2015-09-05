package main

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
	"github.com/paked/gerrycode/communicator"
	"github.com/paked/restrict"
)

var (
	db *sql.DB
)

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"`
}

func (u *User) Login(password string) bool {
	if password == u.Password { // This should be moved to bcrypt at some point
		return true
	}

	return false
}

func main() {
	var err error
	db, err = sql.Open("sqlite3", "database.db")
	if err != nil {
		log.Println("Could not open DB")
		return
	}

	r := mux.NewRouter()

	// User authentication, registration and management
	r.HandleFunc("/users/register", RegisterUserHandler).Methods("POST")
	r.HandleFunc("/users/login", LoginUserHandler).Methods("POST")
	r.HandleFunc("/users/info", restrict.R(InfoHandler)).Methods("GET")

	// Food graph handling
	r.HandleFunc("/foods/graph", FoodGraphHandler).Methods("GET")

	// Pantry management
	r.HandleFunc("/pantry", restrict.R(AddFoodToPantry)).Methods("POST")

	http.Handle("/", r)

	log.Fatal(http.ListenAndServe("localhost:8080", nil))
}

type Graph struct {
	Name   string    `json:"name"`
	Points []float64 `json:"points"`
}

type FoodGraph struct {
	Measurements []Graph `json:"measurements"`
}

func FoodGraphHandler(w http.ResponseWriter, r *http.Request) {
	c := communicator.New(w)
	fg := FoodGraph{
		Measurements: []Graph{
			// ideas: Weight, Eating Frequency, etc
			{Name: "Weight", Points: []float64{90, 89, 87, 87.5}},
			{Name: "Randomo", Points: []float64{22, 35, 44, 25}},
		},
	}

	c.OKWithData("Here is your graph", fg)
}

func InfoHandler(w http.ResponseWriter, r *http.Request, t *jwt.Token) {
	c := communicator.New(w)
	u, err := getUserFromToken(t)
	if err != nil {
		c.Fail("Could not get user ID from token")
		return
	}

	c.OKWithData("Here is your user, welcome to the party", u)
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

	res, err := db.Exec("INSERT INTO users (username, password) VALUES (?, ?)", username, password)
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

func LoginUserHandler(w http.ResponseWriter, r *http.Request) {
	c := communicator.New(w)

	username := r.FormValue("username")
	password := r.FormValue("password")

	u, err := GetUser("username", username)
	if err != nil {
		log.Println(err)
		c.Fail("Could not get user")
		return
	}

	ok := u.Login(password)
	if !ok {
		c.Fail("Incorrect username")
		return
	}

	claims := make(map[string]interface{})
	claims["id"] = u.ID
	claims["exp"] = time.Now().Add(time.Hour * 72).Unix()

	ts, err := restrict.Token(claims)
	if err != nil {
		c.Fail("Failure signing that token!")
		return
	}

	c.OKWithData("Here is your token", ts)
}

func GetUserByID(id int64) (User, error) {
	u := User{}

	row := db.QueryRow("SELECT id, username, password FROM users WHERE id = ?", id)

	err := row.Scan(&u.ID, &u.Username, &u.Password)
	if err != nil {
		return u, err
	}

	return u, nil
}

func GetUser(key, value string) (User, error) {
	u := User{}

	row := db.QueryRow("SELECT id, username, password FROM users WHERE "+key+" = ?", value)
	err := row.Scan(&u.ID, &u.Username, &u.Password)
	if err != nil {
		return u, err
	}

	return u, nil
}

type Stock struct {
	ID     int64
	Name   string
	Weight float64
}

func AddFoodToPantry(w http.ResponseWriter, r *http.Request, t *jwt.Token) {
	c := communicator.New(w)
	name := r.FormValue("name")
	stringWeight := r.FormValue("weight")

	intWeight, err := strconv.Atoi(stringWeight)
	if err != nil {
		c.Fail("Could not convert weight")
		return
	}

	weight := float64(intWeight)

	s := Stock{
		Name:   name,
		Weight: weight,
	}

	res, err := db.Exec("INSERT INTO pantry (name, weight) VALUES (?, ?)", s.Name, s.Weight)
	if err != nil {
		log.Println(err)
		c.Fail("Could not insert food into pantry")
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		c.Fail("Could not get ID from new food")
		return
	}

	s.ID = id

	c.OKWithData("Added food to pantry", s)
}

func getUserFromToken(t *jwt.Token) (User, error) {
	fid, ok := t.Claims["id"].(float64)
	if !ok {
		return User{}, errors.New("Could not get user from token")
	}

	id := int64(fid)

	return GetUserByID(id)
}
