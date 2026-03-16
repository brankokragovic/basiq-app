package models

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type UserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type SubClass struct {
	Code string `json:"code"`
}

type Transaction struct {
	ID        string   `json:"id"`
	Amount    string   `json:"amount"`
	Direction string   `json:"direction"`
	SubClass  SubClass `json:"subClass"`
	PostDate  string   `json:"postDate"`
}

type TransactionList struct {
	Data  []Transaction `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

type JobStep struct {
	Title  string `json:"title"`
	Status string `json:"status"`
	Result *struct {
		URL string `json:"url"`
	} `json:"result,omitempty"`
}

type JobResponse struct {
	ID    string    `json:"id"`
	Steps []JobStep `json:"steps"`
}
