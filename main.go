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

func fetchFeed(url string) ([]AtomEntry, error) {
	resp, err := http.Get(url)
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

func main() {
	url := os.Getenv("FEED_URL")
	if url == "" {
		fmt.Println("FEED_URL environment variable is not set")
		return
	}

	ctx := context.Background()
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		projectID = os.Getenv("GCP_PROJECT")
	}
	if projectID == "" {
		fmt.Println("GCP project ID environment variable is not set")
		return
	}

	databaseID := os.Getenv("FIREBASE_DATABASE_ID")
	if databaseID == "" {
		databaseID = firestore.DefaultDatabaseID
	}
	store, err := newFirestoreStore(ctx, projectID, databaseID)
	if err != nil {
		fmt.Printf("Error creating Firestore client: %v\n", err)
		return
	}
	defer store.Close()

	entries, err := fetchFeed(url)
	if err != nil {
		fmt.Printf("Error fetching feed: %v\n", err)
		return
	}

	headers := []string{
		"Subject: New RFD Gift Card Deals",
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=\"utf-8\"",
	}

	seenIDs, err := store.Load(ctx)
	if err != nil {
		fmt.Printf("Error loading seen IDs from Firestore: %v\n", err)
		return
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
		fmt.Println("No new entries found.")
		return
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

	htmlBody := bodyBuilder.String()
	msg := strings.Join(headers, "\r\n") + "\r\n\r\n" + htmlBody
	smtpHost := os.Getenv("SMTP_HOST")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	if smtpHost == "" || smtpUser == "" || smtpPass == "" {
		fmt.Println("SMTP environment variables are not set")
		return
	}
	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	addr := smtpHost + ":587"
	err = smtp.SendMail(addr, auth, smtpUser, []string{"johnmega999@gmail.com"}, []byte(msg))
	if err != nil {
		fmt.Printf("Error sending email: %v\n", err)
		return
	}

	ids := make([]string, len(newEntries))
	for i, entry := range newEntries {
		ids[i] = entry.ID
	}
	if err := store.Mark(ctx, ids); err != nil {
		fmt.Printf("Error marking entries in Firestore: %v\n", err)
	}
}
