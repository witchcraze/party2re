package casino

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
)

type Suit string

const (
	SuitSpades   Suit = "♠"
	SuitHearts   Suit = "♥"
	SuitDiamonds Suit = "♦"
	SuitClubs    Suit = "♣"
)

type Rank int

const (
	RankAce   Rank = 1
	RankTwo   Rank = 2
	RankThree Rank = 3
	RankFour  Rank = 4
	RankFive  Rank = 5
	RankSix   Rank = 6
	RankSeven Rank = 7
	RankEight Rank = 8
	RankNine  Rank = 9
	RankTen   Rank = 10
	RankJack  Rank = 11
	RankQueen Rank = 12
	RankKing  Rank = 13
)

func (r Rank) String() string {
	switch r {
	case RankAce:
		return "A"
	case RankJack:
		return "J"
	case RankQueen:
		return "Q"
	case RankKing:
		return "K"
	default:
		return fmt.Sprintf("%d", int(r))
	}
}

type Card struct {
	Suit Suit `json:"suit"`
	Rank Rank `json:"rank"`
}

func (c Card) String() string {
	return fmt.Sprintf("%s%s", c.Rank.String(), c.Suit)
}

type Deck struct {
	cards []Card
}

func NewStandardDeck() *Deck {
	suits := []Suit{SuitSpades, SuitHearts, SuitDiamonds, SuitClubs}
	cards := make([]Card, 0, 52)
	for _, s := range suits {
		for r := RankAce; r <= RankKing; r++ {
			cards = append(cards, Card{Suit: s, Rank: r})
		}
	}
	return &Deck{cards: cards}
}

func (d *Deck) Shuffle() error {
	n := len(d.cards)
	for i := n - 1; i > 0; i-- {
		jBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		j := int(jBig.Int64())
		d.cards[i], d.cards[j] = d.cards[j], d.cards[i]
	}
	return nil
}

func (d *Deck) Draw() (Card, error) {
	if len(d.cards) == 0 {
		return Card{}, errors.New("deck is empty")
	}
	card := d.cards[0]
	d.cards = d.cards[1:]
	return card, nil
}

func (d *Deck) Remaining() int {
	return len(d.cards)
}
