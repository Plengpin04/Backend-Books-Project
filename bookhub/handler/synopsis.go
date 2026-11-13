package handler

import (
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	synopsisHTTPClient = &http.Client{
		Timeout: 6 * time.Second,
	}

	htmlTagPattern = regexp.MustCompile(`<[^>]*>`)
)

type googleBooksResponse struct {
	Items []struct {
		ID         string `json:"id"`
		VolumeInfo struct {
			Description         string `json:"description"`
			IndustryIdentifiers []struct {
				Identifier string `json:"identifier"`
			} `json:"industryIdentifiers"`
		} `json:"volumeInfo"`
	} `json:"items"`
}

type openLibraryEdition struct {
	Works []struct {
		Key string `json:"key"`
	} `json:"works"`
}

type openLibraryWork struct {
	Description json.RawMessage `json:"description"`
}

func cleanSynopsis(value string) string {
	value = htmlTagPattern.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	value = strings.Join(strings.Fields(value), " ")

	return strings.TrimSpace(value)
}

func sameISBN(a, b string) bool {
	a = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(a), "-", ""))
	b = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(b), "-", ""))

	return a == b
}

func fetchJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint,
		nil,
	)

	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MoonyBooks/1.0")

	resp, err := synopsisHTTPClient.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}

	return json.NewDecoder(
		io.LimitReader(resp.Body, 2*1024*1024),
	).Decode(target)
}

func getGoogleBooksSynopsis(
	ctx context.Context,
	isbn string,
) (string, string, bool) {
	query := url.Values{}
	query.Set("q", "isbn:"+isbn)
	query.Set("maxResults", "5")

	apiKey := strings.TrimSpace(os.Getenv("GOOGLE_BOOKS_API_KEY"))

	if apiKey != "" {
		query.Set("key", apiKey)
	}

	endpoint := "https://www.googleapis.com/books/v1/volumes?" + query.Encode()

	var result googleBooksResponse

	if err := fetchJSON(ctx, endpoint, &result); err != nil {
		return "", "", false
	}

	for _, item := range result.Items {
		// ตรวจ ISBN ก่อน เพื่อป้องกันการดึงเรื่องย่อผิดเล่ม
		matched := false

		for _, identifier := range item.VolumeInfo.IndustryIdentifiers {
			if sameISBN(identifier.Identifier, isbn) {
				matched = true
				break
			}
		}

		if !matched {
			continue
		}

		description := cleanSynopsis(item.VolumeInfo.Description)

		if description == "" {
			continue
		}

		sourceURL := ""

		if item.ID != "" {
			sourceURL = "https://books.google.com/books?id=" +
				url.QueryEscape(item.ID)
		}

		return description, sourceURL, true
	}

	return "", "", false
}

func getOpenLibrarySynopsis(
	ctx context.Context,
	isbn string,
) (string, string, bool) {
	editionURL := "https://openlibrary.org/isbn/" +
		url.PathEscape(isbn) + ".json"

	var edition openLibraryEdition

	if err := fetchJSON(ctx, editionURL, &edition); err != nil {
		return "", "", false
	}

	for _, work := range edition.Works {
		if work.Key == "" {
			continue
		}

		workURL := "https://openlibrary.org" + work.Key + ".json"

		var result openLibraryWork

		if err := fetchJSON(ctx, workURL, &result); err != nil {
			continue
		}

		description := ""

		// Open Library อาจส่ง description เป็น string
		// หรือเป็น object ที่มี field value
		var plainText string

		if err := json.Unmarshal(result.Description, &plainText); err == nil {
			description = plainText
		} else {
			var object struct {
				Value string `json:"value"`
			}

			if err := json.Unmarshal(result.Description, &object); err == nil {
				description = object.Value
			}
		}

		description = cleanSynopsis(description)

		if description == "" {
			continue
		}

		return description, workURL, true
	}

	return "", "", false
}

func lookupBookSynopsis(
	ctx context.Context,
	isbn string,
) (string, string, string) {
	// แหล่งแรก: Google Books
	if description, sourceURL, ok := getGoogleBooksSynopsis(ctx, isbn); ok {
		return description, "Google Books", sourceURL
	}

	// แหล่งสำรอง: Open Library
	if description, sourceURL, ok := getOpenLibrarySynopsis(ctx, isbn); ok {
		return description, "Open Library", sourceURL
	}

	return "", "", ""
}
