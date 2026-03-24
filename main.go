package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"os"
	"strings"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

type AtomLink struct {
	Href string `xml:"href,attr"`
}

type AtomContent struct {
	Title string `xml:"type,attr"`
	Body  string `xml:",chardata"`
	Base  string `xml:"http://www.w3.org/XML/1998/namespace base,attr"`
}

type AtomEntry struct {
	Title   string      `xml:"title"`
	Link    AtomLink    `xml:"link"`
	Updated string      `xml:"updated"`
	Content AtomContent `xml:"content"`
	ID      string      `xml:"id"`
}

type Item struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	PubDate string `xml:"pubDate"`
	GUID    string `xml:"guid"`
}

type AtomFeed struct {
	Entries []AtomEntry `xml:"entry"`
}

type FirestoreStore struct {
	client *firestore.Client
	coll   *firestore.CollectionRef
}

type Config struct {
	FeedURL    string
	ProjectID  string
	DatabaseID string
	SMTPHost   string
	SMTPUser   string
	SMTPPass   string
	EmailTo    string
}

func newFirestoreStore(ctx context.Context, projectID, databaseID string) (*FirestoreStore, error) {
	var client *firestore.Client
	var err error

	switch {
	case databaseID == "" || databaseID == firestore.DefaultDatabaseID:
		client, err = firestore.NewClient(ctx, projectID)
	default:
		client, err = firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	}

	if err != nil {
		return nil, fmt.Errorf("firestore.NewClient: %w", err)
	}

	return &FirestoreStore{
		client: client,
		coll:   client.Collection("seen_entries"),
	}, nil
}

func (s *FirestoreStore) Close() error {
	return s.client.Close()
}

func (s *FirestoreStore) Load(ctx context.Context) (map[string]struct{}, error) {
	seen := make(map[string]struct{})
	iter := s.coll.Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("firestore iterator: %w", err)
		}
		seen[doc.Ref.ID] = struct{}{}
	}

	return seen, nil
}

func (s *FirestoreStore) Mark(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	batch := s.client.Batch()
	for _, id := range ids {
		doc := s.coll.Doc(id)
		batch.Set(doc, map[string]any{"seenAt": firestore.ServerTimestamp}, firestore.MergeAll)
	}

	_, err := batch.Commit(ctx)
	return err
}

func fetchFeed(ctx context.Context, url string) ([]AtomEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch feed: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var feed AtomFeed
	err = xml.Unmarshal(body, &feed)
	if err != nil {
		return nil, err
	}
	return feed.Entries, nil
}

func loadConfig() (Config, error) {
	cfg := Config{
		FeedURL:    os.Getenv("FEED_URL"),
		ProjectID:  os.Getenv("GOOGLE_CLOUD_PROJECT"),
		DatabaseID: os.Getenv("FIREBASE_DATABASE_ID"),
		SMTPHost:   os.Getenv("SMTP_HOST"),
		SMTPUser:   os.Getenv("SMTP_USER"),
		SMTPPass:   os.Getenv("SMTP_PASS"),
		EmailTo:    os.Getenv("EMAIL_TO"),
	}

	if cfg.ProjectID == "" {
		cfg.ProjectID = os.Getenv("GCP_PROJECT")
	}
	if cfg.DatabaseID == "" {
		cfg.DatabaseID = firestore.DefaultDatabaseID
	}
	if cfg.EmailTo == "" {
		cfg.EmailTo = "johnmega999@gmail.com"
	}

	switch {
	case cfg.FeedURL == "":
		return Config{}, fmt.Errorf("FEED_URL environment variable is not set")
	case cfg.ProjectID == "":
		return Config{}, fmt.Errorf("GCP project ID environment variable is not set")
	case cfg.SMTPHost == "" || cfg.SMTPUser == "" || cfg.SMTPPass == "":
		return Config{}, fmt.Errorf("SMTP environment variables are not set")
	default:
		return cfg, nil
	}
}

func processFeed(ctx context.Context, store *FirestoreStore, cfg Config) (string, error) {
	entries, err := fetchFeed(ctx, cfg.FeedURL)
	if err != nil {
		return "", fmt.Errorf("fetching feed: %w", err)
	}

	seenIDs, err := store.Load(ctx)
	if err != nil {
		return "", fmt.Errorf("loading seen IDs from Firestore: %w", err)
	}

	var newEntries []AtomEntry
	for _, entry := range entries {
		if _, seen := seenIDs[entry.ID]; seen {
			continue
		}
		seenIDs[entry.ID] = struct{}{}
		newEntries = append(newEntries, entry)
	}

	if len(newEntries) == 0 {
		return "No new entries found.", nil
	}

	headers := []string{
		"Subject: New RFD Gift Card Deals",
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=\"utf-8\"",
	}

	var bodyBuilder strings.Builder
	bodyBuilder.WriteString("<html><body><h1>New Feed Items</h1><ul>")
	for _, entry := range newEntries {
		bodyBuilder.WriteString("<article>\n")
		bodyBuilder.WriteString(fmt.Sprintf("<li><a href=\"%s\">%s</a><br>%s</li>",
			entry.Link.Href, entry.Title, entry.Content.Body))
		bodyBuilder.WriteString("\n</article>\n<hr>\n")
	}
	bodyBuilder.WriteString("</ul></body></html>")

	msg := strings.Join(headers, "\r\n") + "\r\n\r\n" + bodyBuilder.String()
	auth := smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPHost)
	addr := cfg.SMTPHost + ":587"
	if err := smtp.SendMail(addr, auth, cfg.SMTPUser, []string{cfg.EmailTo}, []byte(msg)); err != nil {
		return "", fmt.Errorf("sending email: %w", err)
	}

	ids := make([]string, len(newEntries))
	for i, entry := range newEntries {
		ids[i] = entry.ID
	}
	if err := store.Mark(ctx, ids); err != nil {
		return "", fmt.Errorf("marking entries in Firestore: %w", err)
	}

	return fmt.Sprintf("Processed %d new entries.", len(newEntries)), nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Println(err)
		return
	}

	ctx := context.Background()
	store, err := newFirestoreStore(ctx, cfg.ProjectID, cfg.DatabaseID)
	if err != nil {
		fmt.Printf("Error creating Firestore client: %v\n", err)
		return
	}
	defer store.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		result, err := processFeed(r.Context(), store, cfg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(result))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Listening on port %s\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		fmt.Printf("HTTP server failed: %v\n", err)
	}
}
