package nexusggr

import "time"

type Config struct {
	BaseURL    string
	AgentCode  string
	AgentToken string
	Timeout    time.Duration
}

type Provider struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Status int    `json:"status"`
}

type Game struct {
	GameCode string `json:"game_code"`
	GameName string `json:"game_name"`
	Banner   string `json:"banner"`
	Status   int    `json:"status"`
}

type ProviderListResult struct {
	Message   string     `json:"msg"`
	Providers []Provider `json:"providers"`
}

type GameListResult struct {
	Message string `json:"msg"`
	Games   []Game `json:"games"`
}

type GameLaunchInput struct {
	UserCode     string
	ProviderCode string
	GameCode     string
	Lang         string
}

type GameLaunchResult struct {
	Message   string `json:"msg"`
	LaunchURL string `json:"launch_url"`
}

type MoneyInfoInput struct {
	UserCode string
	AllUsers bool
}

type Balance struct {
	UserCode string  `json:"user_code"`
	Balance  float64 `json:"balance"`
}

type AgentBalance struct {
	AgentCode string  `json:"agent_code"`
	Balance   float64 `json:"balance"`
}

type MoneyInfoResult struct {
	Message string       `json:"msg"`
	Agent   AgentBalance `json:"agent"`
	User    *Balance     `json:"user,omitempty"`
	Users   []Balance    `json:"user_list,omitempty"`
}

type UserCreateInput struct {
	UserCode string
}

type UserCreateResult struct {
	Message     string  `json:"msg"`
	UserCode    string  `json:"user_code"`
	UserBalance float64 `json:"user_balance"`
}

type TransferInput struct {
	UserCode  string
	Amount    float64
	AgentSign string
}

type TransferResult struct {
	Message      string  `json:"msg"`
	AgentBalance float64 `json:"agent_balance"`
	UserBalance  float64 `json:"user_balance"`
}

type UserWithdrawResetInput struct {
	UserCode string
	AllUsers bool
}

type UserWithdrawResetUser struct {
	UserCode       string  `json:"user_code"`
	WithdrawAmount float64 `json:"withdraw_amount"`
	Balance        float64 `json:"balance"`
}

type UserWithdrawResetResult struct {
	Message string                  `json:"msg"`
	Agent   AgentBalance            `json:"agent"`
	User    *UserWithdrawResetUser  `json:"user,omitempty"`
	Users   []UserWithdrawResetUser `json:"user_list,omitempty"`
}

type TransferStatusInput struct {
	UserCode  string
	AgentSign string
}

type TransferStatusResult struct {
	Message      string  `json:"msg"`
	Amount       float64 `json:"amount"`
	AgentBalance float64 `json:"agent_balance"`
	UserBalance  float64 `json:"user_balance"`
	Type         string  `json:"type"`
}
