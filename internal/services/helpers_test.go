package services

import "testing"

func TestAmountOK(t *testing.T) {
 if !amountOK(1) || !amountOK(100000) { t.Fatal("positive minor-unit amounts should pass") }
 if amountOK(0) || amountOK(-1) { t.Fatal("zero and negative amounts must fail") }
}
