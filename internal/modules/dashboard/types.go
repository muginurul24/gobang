package dashboard

type Summary struct {
	Role            string           `json:"role"`
	StoreMetrics    *StoreMetrics    `json:"store_metrics,omitempty"`
	PlatformMetrics *PlatformMetrics `json:"platform_metrics,omitempty"`
}

type StoreMetrics struct {
	AccessibleStoreCount int    `json:"accessible_store_count"`
	ActiveStoreCount     int    `json:"active_store_count"`
	LowBalanceStoreCount int    `json:"low_balance_store_count"`
	BalanceTotal         string `json:"balance_total"`
	PendingQRISCount     int    `json:"pending_qris_count"`
	SuccessTodayCount    int    `json:"success_today_count"`
	ExpiredTodayCount    int    `json:"expired_today_count"`
	MonthlyStoreIncome   string `json:"monthly_store_income"`
}

type PlatformMetrics struct {
	PlatformIncomeToday    string  `json:"platform_income_today"`
	PlatformIncomeMonth    string  `json:"platform_income_month"`
	TotalStoreCount        int     `json:"total_store_count"`
	ActiveStoreCount       int     `json:"active_store_count"`
	LowBalanceStoreCount   int     `json:"low_balance_store_count"`
	PendingWithdrawCount   int     `json:"pending_withdraw_count"`
	UpstreamErrorRate24h   float64 `json:"upstream_error_rate_24h"`
	CallbackFailureRate24h float64 `json:"callback_failure_rate_24h"`
}
