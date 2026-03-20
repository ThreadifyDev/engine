package billing

import (
	"threadify-go/shared/database"
)

type CompanyKeys struct {
	Balance string `json:"balance"`
	Charged string `json:"charged"`
	Pending string `json:"pending"`
}

func KeysFor(companyID string) CompanyKeys {
	return CompanyKeys{
		Balance: database.CreditBalanceKeyPrefix + companyID,
		Charged: database.CreditMonthlyChargedKeyPrefix + companyID,
		Pending: database.CreditTopupPendingKeyPrefix + companyID,
	}
}
