package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
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
	"github.com/sfreiberg/gotwilio"
)

var (
	db *sql.DB

	twilio *gotwilio.Twilio
	from   string
)

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"`
	Phone    string `json:"phone"`
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

	//Twilio Config
	from = "+14847184408"
	accountSid := "AC800a64542126d28255c7c82aa375627f"
	authToken := "f8c3c917be8b7ec2225a6066eff08719"
	twilio = gotwilio.NewTwilioClient(accountSid, authToken)

	r := mux.NewRouter()

	// User authentication, registration and management
	r.HandleFunc("/users/register", RegisterUserHandler).Methods("POST")
	r.HandleFunc("/users/login", LoginUserHandler).Methods("POST")
	r.HandleFunc("/users/info", restrict.R(InfoHandler)).Methods("GET")

	// Food graph handling
	r.HandleFunc("/foods/graph", FoodGraphHandler).Methods("GET")

	// Pantry management
	r.HandleFunc("/pantry", restrict.R(RegisterFoodHandler)).Methods("POST")
	r.HandleFunc("/pantry", restrict.R(AllFoodInPantryHandler)).Methods("GET")
	r.HandleFunc("/pantry/consume", restrict.R(ConsumeFoodHandler)).Methods("POST")

	http.Handle("/", r)

	log.Fatal(http.ListenAndServe(":8080", nil))
}

func AllFoodInPantryHandler(w http.ResponseWriter, r *http.Request, t *jwt.Token) {
	c := communicator.New(w)

	u, err := getUserFromToken(t)
	if err != nil {
		c.Fail("Could not get token from user")
		return
	}

	st := []Stock{}

	rows, err := db.Query("SELECT id, brand, category, manufacturer, description FROM pantry WHERE user = ?", u.ID)

	for rows.Next() {
		s := Stock{}

		err = rows.Scan(&s.ID, &s.Brand, &s.Category, &s.Manufacturer, &s.Description)
		if err != nil {
			log.Println(err)

			c.Fail("Could not get information out of ID")
			return
		}

		st = append(st, s)
	}

	c.OKWithData("food.", st)
}

// adds a new data point to relevant statistics
// removes food items that have "ran out of stock"
func ConsumeFoodHandler(w http.ResponseWriter, r *http.Request, t *jwt.Token) {
	c := communicator.New(w)

	u, err := getUserFromToken(t)
	if err != nil {
		c.Fail("Could not get user from token")
		return
	}

	idString := r.FormValue("food_item")
	quantityString := r.FormValue("quantity")

	idInt, err := strconv.Atoi(idString)
	if err != nil {
		c.Fail("That id is not int convertable")
		return
	}

	quantityInt, err := strconv.Atoi(quantityString)
	if err != nil {
		c.Fail("That qunaitty is not convertable")
		return
	}

	id := int64(idInt)
	quantity := float64(quantityInt)
	log.Println(quantity) // TODO use quantity
	s := Stock{}

	row := db.QueryRow("SELECT id, user, brand, category, manufacturer, description FROM pantry WHERE id = ? AND user = ?", id, u.ID)
	err = row.Scan(&s.ID, &s.Category, &s.Manufacturer, &s.Description)
	if err != nil {
		log.Println(err)
		c.Fail("Could not get that db")
		return
	}
	/*
		_, err = db.Exec("INSERT INTO stats_values (value, corresponds, user) VALUES (?, 0, ?)", s.Calories, u.ID)
		if err != nil {
			log.Println(err)
			c.Fail("Could not insert stats")
			return
		}*/
	/*
		_, err = db.Exec("UPDATE pantry SET weight = ? AND avail = ? WHERE id = ? AND user = ?", s.Weight-quantity, (s.Weight-quantity) >= 0, s.ID, u.ID)
		if err != nil {
			log.Println(err)
			c.Fail("Could not update pantry")
			return
		}*/
	/*
		if s.Weight-quantity < 2 {
			message := "You are running out of " + s.Name
			twilio.SendSMS(from, u.Phone, message, "", "")
		}*/

	c.OK("consumed.")
}

/*
type Stock struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Weight      float64 `json:"weight"`
	Calories    float64 `json:"calories"`
	Cholesterol float64 `json:"cholesterol"`
}

type stock struct {
	Name          string  `json:"item_name"`
	TotalServes   float64 `json:"nf_servings_per_container"`
	ServingWeight float64 `json:"nf_serving_weight_grams"`
	Calories      float64 `json:"nf_calories"`
	Cholesterol   float64 `json:"nf_cholesterol"`
}*/

type UPCRequest struct {
	Authentication string            `json:"auth"`
	Method         string            `json:"method"`
	Parameters     map[string]string `json:"params"`
}

type UPCResponse struct {
	Result  Stock `json:"result"`
	Success bool  `json:"success"`
}

type Stock struct {
	ID           int64  `json:"id"`
	Brand        string `json:"brand"`
	Category     string `json:"category"`
	Manufacturer string `json:"manufacturer"`
	Description  string `json:"description"`
	UPC          string `json:"upc"`
}

// Pass in UPC number of new food
// * Fetch food info
// * Cache
func RegisterFoodHandler(w http.ResponseWriter, r *http.Request, t *jwt.Token) {
	c := communicator.New(w)
	s := Stock{}

	u, err := getUserFromToken(t)
	if err != nil {
		c.Fail("Could not get user from token")
		return
	}

	log.Println(u, s)

	upc := r.FormValue("upc") // send upc to nutritionix

	upcReq := UPCRequest{
		Authentication: "Jad19r2OAfrNHpZH2BcuOZQUXDTLhcrS",
		Method:         "FetchProductByUPC",
		Parameters:     map[string]string{"upc": upc},
	}

	jsonStr, err := json.Marshal(upcReq)
	if err != nil {
		c.Fail("COuld not marshall req")
		return
	}

	url := "http://api.simpleupc.com/v1.php"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	cli := http.Client{}
	resp, err := cli.Do(req)
	upcResp := UPCResponse{}

	err = json.NewDecoder(resp.Body).Decode(&upcResp)
	if err != nil {
		c.Fail("Could not decode JSON")
		return
	}

	if !upcResp.Success {
		c.Fail("You what is up you failed!")
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		s := buf.String()
		log.Println(s)
		return
	}

	log.Println(upcResp.Result)
	s = upcResp.Result

	res, err := db.Exec("INSERT INTO pantry (user, brand, category, manufacturer, description) VALUES (?, ?, ?, ?, ?)", u.ID, s.Brand, s.Category, s.Manufacturer, s.Description)
	if err != nil {
		log.Println(err)
		c.Fail("Kill the pantry")
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		c.Fail("Could not get last insert id")
		return
	}

	s.ID = id

	c.OKWithData("Here is your res", s)
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

func getUserFromToken(t *jwt.Token) (User, error) {
	fid, ok := t.Claims["id"].(float64)
	if !ok {
		return User{}, errors.New("Could not get user from token")
	}

	id := int64(fid)

	return GetUserByID(id)
}
