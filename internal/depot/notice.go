package depot

import (
	"context"
	"errors"
	"fmt"

	"github.com/witchcraze/party2re/internal/core/item"
)

const (
	NoticeItemTransfer  = "item_transfer"
	NoticeMoneyTransfer = "money_transfer"
)

// DeliveryNotifier records recipient Home notices inside the caller's transaction.
type DeliveryNotifier interface {
	AddDeliveryNotice(ctx context.Context, characterID, noticeType, message string) error
	// AddDeliveryFlag skips the notice while one of the same type is uncleared.
	AddDeliveryFlag(ctx context.Context, characterID, noticeType, message string) error
}

// SetDeliveryNotifier enables recipient notices for sends. Configure before
// serving requests; without one, sends record no notice.
func (s *Service) SetDeliveryNotifier(notifier DeliveryNotifier) {
	s.notifier = notifier
}

func (s *Service) notifyMoneySent(ctx context.Context, recipientID string) error {
	if s.notifier == nil {
		return nil
	}
	return s.notifier.AddDeliveryFlag(ctx, recipientID, NoticeMoneyTransfer, "お金が預かり所に届いています")
}

func (s *Service) notifyItemSent(ctx context.Context, recipientID, senderName string, inst item.Instance) error {
	if s.notifier == nil {
		return nil
	}
	if s.itemDefs == nil {
		return errors.New("depot item notices require item definitions")
	}
	def, err := s.itemDefs.FindByID(inst.DefinitionID)
	if err != nil {
		return fmt.Errorf("notice item %s: %w", inst.DefinitionID, err)
	}
	return s.notifier.AddDeliveryNotice(ctx, recipientID, NoticeItemTransfer,
		fmt.Sprintf("預かり所に%sから%sが届いています", senderName, def.Name))
}
