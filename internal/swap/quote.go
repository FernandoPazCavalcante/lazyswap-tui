package swap

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// PriceProvider abstracts the DEX call so Quote can be tested without RPC.
// Implementations: dex.UniswapV2Provider.
type PriceProvider interface {
	GetPrice(ctx context.Context, fromToken, toToken, inputAmount string) (string, error)
}

// QuoteResult is the output of Quote.Get.
type QuoteResult struct {
	EstimatedOutput string
	FromToken       string
	ToToken         string
	InputAmount     string
}

// Quote validates inputs and delegates pricing to a PriceProvider.
// Mirrors swap-quote.service.ts.
type Quote struct {
	provider PriceProvider
}

// NewQuote wires the provider.
func NewQuote(p PriceProvider) *Quote { return &Quote{provider: p} }

// Get returns a quote for swapping inputAmount of fromToken into toToken.
func (q *Quote) Get(ctx context.Context, fromToken, toToken, inputAmount string) (QuoteResult, error) {
	if strings.TrimSpace(fromToken) == "" {
		return QuoteResult{}, errors.New("invalid fromToken address")
	}
	if strings.TrimSpace(toToken) == "" {
		return QuoteResult{}, errors.New("invalid toToken address")
	}
	v, err := strconv.ParseFloat(inputAmount, 64)
	if err != nil {
		return QuoteResult{}, errors.New("invalid amount format")
	}
	if v <= 0 {
		return QuoteResult{}, errors.New("amount must be greater than zero")
	}

	out, err := q.provider.GetPrice(ctx, fromToken, toToken, inputAmount)
	if err != nil {
		return QuoteResult{}, err
	}
	return QuoteResult{
		EstimatedOutput: out,
		FromToken:       fromToken,
		ToToken:         toToken,
		InputAmount:     inputAmount,
	}, nil
}
