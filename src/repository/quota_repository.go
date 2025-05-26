package repository

import (
	"errors"
	"log"
	"project/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// QuotaRepository defines the interface for interacting with guest quota data.
type QuotaRepository interface {
	GetQuota(guestUserID string) (*models.GuestQuota, error)
	IncrementQuota(guestUserID string) (*models.GuestQuota, error)
	// ResetQuota(guestUserID string) error // Optional, can be added if needed for testing or admin purposes
}

type quotaRepository struct {
	db *gorm.DB
}

// NewQuotaRepository creates a new instance of QuotaRepository.
func NewQuotaRepository(db *gorm.DB) QuotaRepository {
	// Auto-migrate the schema for GuestQuota model
	// It's often done in main.go, but can also be ensured here if this repo is the sole manager.
	// However, to avoid multiple migration attempts for the same table if other parts of the app also call it,
	// it's generally better to centralize AutoMigrate calls.
	// For this task, we'll assume AutoMigrate is handled in main.go or a similar central place.
	// err := db.AutoMigrate(&models.GuestQuota{})
	// if err != nil {
	// 	log.Fatalf("Failed to auto-migrate GuestQuota table: %v", err)
	// }
	return &quotaRepository{db: db}
}

// GetQuota retrieves the current quota usage for a guest user.
// If the guest user is not found, it returns a new GuestQuota object with 0 messages sent
// and no error. This is a specific behavior for quota checking.
func (r *quotaRepository) GetQuota(guestUserID string) (*models.GuestQuota, error) {
	if guestUserID == "" {
		log.Printf("ERROR: [QuotaRepository] GetQuota: guestUserID cannot be empty.")
		return nil, errors.New("guest user ID cannot be empty")
	}
	log.Printf("INFO: [QuotaRepository] Attempting to get quota for guestUserID: %s", guestUserID)

	var quota models.GuestQuota
	err := r.db.First(&quota, "guest_user_id = ?", guestUserID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("INFO: [QuotaRepository] No quota record found for guestUserID %s. Returning new quota with 0 messages sent.", guestUserID)
			return &models.GuestQuota{GuestUserID: guestUserID, MessagesSent: 0}, nil
		}
		log.Printf("ERROR: [QuotaRepository] Failed to fetch quota for guestUserID %s: %v", guestUserID, err)
		return nil, fmt.Errorf("failed to fetch quota for guestUserID %s: %w", guestUserID, err)
	}
	log.Printf("INFO: [QuotaRepository] Successfully fetched quota for guestUserID %s: %d messages sent.", guestUserID, quota.MessagesSent)
	return &quota, nil
}

// IncrementQuota increments the message count for a guest user.
// If the user doesn't exist, it creates a new record. Uses GORM's OnConflict (UPSERT).
func (r *quotaRepository) IncrementQuota(guestUserID string) (*models.GuestQuota, error) {
	if guestUserID == "" {
		log.Printf("ERROR: [QuotaRepository] IncrementQuota: guestUserID cannot be empty.")
		return nil, errors.New("guest user ID cannot be empty")
	}
	log.Printf("INFO: [QuotaRepository] Attempting to increment quota for guestUserID: %s", guestUserID)

	// The GuestUserID is the primary key, so GORM's Create with OnConflict will handle UPSERT.
	// For a new record, MessagesSent will be 1.
	// For an existing record, messages_sent will be incremented by 1.
	quotaToUpsert := models.GuestQuota{
		GuestUserID:  guestUserID,
		MessagesSent: 1, // This is for the INSERT part of UPSERT
	}

	err := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "guest_user_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{"messages_sent": gorm.Expr("messages_sent + 1")}),
	}).Create(&quotaToUpsert).Error
	// Note: Create(&quotaToUpsert) will perform an INSERT. If conflict on guest_user_id, it will then perform the DoUpdates.
	// The struct quotaToUpsert itself might not be updated with the incremented value if the record already existed.
	// So, we must re-fetch to get the actual current state.

	if err != nil {
		log.Printf("ERROR: [QuotaRepository] Failed to increment quota for guestUserID %s during UPSERT: %v", guestUserID, err)
		return nil, fmt.Errorf("failed to increment quota for guestUserID %s: %w", guestUserID, err)
	}

	// Fetch the record to return the updated/created quota.
	var currentQuota models.GuestQuota
	if fetchErr := r.db.First(&currentQuota, "guest_user_id = ?", guestUserID).Error; fetchErr != nil {
		log.Printf("ERROR: [QuotaRepository] Failed to fetch quota for guestUserID %s after increment: %v", guestUserID, fetchErr)
		return nil, fmt.Errorf("failed to fetch quota for guestUserID %s after increment: %w", guestUserID, fetchErr)
	}

	log.Printf("INFO: [QuotaRepository] Successfully incremented quota for guestUserID %s. New count: %d", guestUserID, currentQuota.MessagesSent)
	return &currentQuota, nil
}
