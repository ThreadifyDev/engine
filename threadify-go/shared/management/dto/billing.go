package dto

import "time"

type UpdateMaxMonthlyRequest struct {
	MaxMonthlyChargeMillicents int64 `json:"max_monthly_millicents" binding:"min=0"`
}

type CheckoutRequest struct {
	AmountMillicents *int64 `json:"amount_millicents" binding:"required"`
}

type CreditAccount struct {
	ID                         string    `json:"id"`
	CompanyID                  string    `json:"company_id"`
	BillingCycleStart          time.Time `json:"billing_cycle_start"`
	BalanceMillicents          int64     `json:"balance_millicents"`
	MinBalanceMillicents       int64     `json:"min_balance_millicents"`
	MaxMonthlyChargeMillicents int64     `json:"max_monthly_charge_millicents"`
	AutoTopupMillicents        int64     `json:"auto_topup_millicents"`
	MonthlyChargedMillicents   int64     `json:"monthly_charged_millicents"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

type GetCurrentPlanResponse struct {
	CreditAccount *CreditAccount `json:"credit_account"`
}

type CheckoutResponse struct {
	URL string `json:"url"`
}

type SuccessResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}
