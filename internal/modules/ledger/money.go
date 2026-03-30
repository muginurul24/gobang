package ledger

import (
	"fmt"
	"strconv"
	"strings"
)

type money int64

func parseMoney(raw string) (money, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, ErrInvalidAmount
	}

	if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return 0, ErrInvalidAmount
	}

	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return 0, ErrInvalidAmount
	}

	wholePart := parts[0]
	if wholePart == "" {
		return 0, ErrInvalidAmount
	}

	if !allDigits(wholePart) {
		return 0, ErrInvalidAmount
	}

	fractionPart := "00"
	if len(parts) == 2 {
		if parts[1] == "" || len(parts[1]) > 2 || !allDigits(parts[1]) {
			return 0, ErrInvalidAmount
		}

		fractionPart = parts[1]
		if len(fractionPart) == 1 {
			fractionPart += "0"
		}
	}

	whole, err := strconv.ParseInt(wholePart, 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}

	fraction, err := strconv.ParseInt(fractionPart, 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}

	return money(whole*100 + fraction), nil
}

func mustParseMoney(raw string) money {
	parsed, err := parseMoney(raw)
	if err != nil {
		panic(err)
	}

	return parsed
}

func (m money) String() string {
	sign := ""
	value := int64(m)
	if value < 0 {
		sign = "-"
		value = -value
	}

	return fmt.Sprintf("%s%d.%02d", sign, value/100, value%100)
}

func (m money) Add(other money) money {
	return m + other
}

func (m money) Sub(other money) money {
	return m - other
}

func (m money) LessThan(other money) bool {
	return m < other
}

func (m money) Equal(other money) bool {
	return m == other
}

func allDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}
