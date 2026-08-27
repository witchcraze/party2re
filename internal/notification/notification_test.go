package notification

import (
	"strings"
	"testing"
)

func TestValidateNewsArticle(t *testing.T) {
	tests := []struct {
		name      string
		category  string
		title     string
		content   string
		author    string
		expectErr error
	}{
		{
			name:      "valid article",
			category:  CategoryAnnouncement,
			title:     "Version 1.0 Released",
			content:   "The game has officially entered version 1.0.",
			author:    "System",
			expectErr: nil,
		},
		{
			name:      "empty title",
			category:  CategoryAnnouncement,
			title:     "   ",
			content:   "Some content",
			author:    "System",
			expectErr: ErrEmptyTitle,
		},
		{
			name:      "title too long",
			category:  CategoryAnnouncement,
			title:     strings.Repeat("a", MaxTitleLength+1),
			content:   "Some content",
			author:    "System",
			expectErr: ErrTitleTooLong,
		},
		{
			name:      "empty content",
			category:  CategoryAnnouncement,
			title:     "Title",
			content:   "   ",
			author:    "System",
			expectErr: ErrEmptyContent,
		},
		{
			name:      "content too long",
			category:  CategoryAnnouncement,
			title:     "Title",
			content:   strings.Repeat("a", MaxContentLength+1),
			author:    "System",
			expectErr: ErrContentTooLong,
		},
		{
			name:      "empty category defaults or handles gracefully",
			category:  "",
			title:     "Title",
			content:   "Some content",
			author:    "System",
			expectErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNewsArticle(tt.category, tt.title, tt.content, tt.author)
			if tt.expectErr != nil {
				if err != tt.expectErr {
					t.Fatalf("expected error %v, got %v", tt.expectErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestValidateNotification(t *testing.T) {
	tests := []struct {
		name      string
		playerID  string
		category  string
		title     string
		body      string
		expectErr error
	}{
		{
			name:      "valid notification",
			playerID:  "p123",
			category:  NotificationCategorySystem,
			title:     "Reward received",
			body:      "You received 100 gold from adventure.",
			expectErr: nil,
		},
		{
			name:      "empty player ID",
			playerID:  "   ",
			category:  NotificationCategorySystem,
			title:     "Title",
			body:      "Body",
			expectErr: ErrInvalidPlayerID,
		},
		{
			name:      "empty title",
			playerID:  "p123",
			category:  NotificationCategorySystem,
			title:     "",
			body:      "Body",
			expectErr: ErrEmptyTitle,
		},
		{
			name:      "title too long",
			playerID:  "p123",
			category:  NotificationCategorySystem,
			title:     strings.Repeat("a", MaxTitleLength+1),
			body:      "Body",
			expectErr: ErrTitleTooLong,
		},
		{
			name:      "empty body",
			playerID:  "p123",
			category:  NotificationCategorySystem,
			title:     "Title",
			body:      "   ",
			expectErr: ErrEmptyBody,
		},
		{
			name:      "body too long",
			playerID:  "p123",
			category:  NotificationCategorySystem,
			title:     "Title",
			body:      strings.Repeat("a", MaxBodyLength+1),
			expectErr: ErrBodyTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNotification(tt.playerID, tt.category, tt.title, tt.body)
			if tt.expectErr != nil {
				if err != tt.expectErr {
					t.Fatalf("expected error %v, got %v", tt.expectErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}
