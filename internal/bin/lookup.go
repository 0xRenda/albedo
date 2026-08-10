package bin

import (
	"albedo-checker/internal/database"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Lookup struct {
	db *database.DB
}

func NewLookup(db *database.DB) *Lookup {
	return &Lookup{db: db}
}

type BINResponse struct {
	Scheme      string `json:"scheme"`
	Type        string `json:"type"`
	Brand       string `json:"brand"`
	Prepaid     bool   `json:"prepaid"`
	Country     struct {
		Name  string `json:"name"`
		Emoji string `json:"emoji"`
	} `json:"country"`
	Bank struct {
		Name string `json:"name"`
	} `json:"bank"`
}

func (l *Lookup) Lookup(bin string) (brand, cardType, level, bank, country, emoji string, err error) {
	if len(bin) < 6 {
		return "", "", "", "", "", "", fmt.Errorf("BIN too short")
	}
	bin = bin[:6]

	cached, err := l.db.GetBINCache(bin)
	if err == nil && cached != nil {
		return cached.Brand, cached.Type, cached.Level, cached.Bank, cached.Country, cached.CountryEmoji, nil
	}

	url := fmt.Sprintf("https://lookup.binlist.net/%s", bin)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept-Version", "3")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", "", "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", "", "", "", "", fmt.Errorf("binlist returned %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var br BINResponse
	if err := json.Unmarshal(body, &br); err != nil {
		return "", "", "", "", "", "", err
	}

	brand = br.Scheme
	cardType = br.Type
	level = br.Brand
	bank = br.Bank.Name
	country = br.Country.Name
	emoji = br.Country.Emoji

	l.db.SetBINCache(bin, brand, cardType, level, bank, country, emoji)
	return brand, cardType, level, bank, country, emoji, nil
}
