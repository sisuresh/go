package processors

import (
	"context"
	"fmt"

	"github.com/stellar/go/ingest"
	"github.com/stellar/go/services/horizon/internal/db2/history"
	"github.com/stellar/go/support/errors"
	"github.com/stellar/go/xdr"
)

type ClaimableBalancesChangeProcessor struct {
	encodingBuffer                *xdr.EncodingBuffer
	qClaimableBalances            history.QClaimableBalances
	cache                         *ingest.ChangeCompactor
	claimantsInsertBuilder        history.ClaimableBalanceClaimantBatchInsertBuilder
	claimableBalanceInsertBuilder history.ClaimableBalanceBatchInsertBuilder
}

func NewClaimableBalancesChangeProcessor(Q history.QClaimableBalances) *ClaimableBalancesChangeProcessor {
	p := &ClaimableBalancesChangeProcessor{
		encodingBuffer:     xdr.NewEncodingBuffer(),
		qClaimableBalances: Q,
	}
	p.reset()
	return p
}

func (p *ClaimableBalancesChangeProcessor) Name() string {
	return "processors.ClaimableBalancesChangeProcessor"
}

func (p *ClaimableBalancesChangeProcessor) reset() {
	p.cache = ingest.NewChangeCompactor()
	p.claimantsInsertBuilder = p.qClaimableBalances.NewClaimableBalanceClaimantBatchInsertBuilder()
	p.claimableBalanceInsertBuilder = p.qClaimableBalances.NewClaimableBalanceBatchInsertBuilder()
}

func (p *ClaimableBalancesChangeProcessor) ProcessChange(ctx context.Context, change ingest.Change) error {
	if change.Type != xdr.LedgerEntryTypeClaimableBalance {
		return nil
	}

	err := p.cache.AddChange(change)
	if err != nil {
		return errors.Wrap(err, "error adding to ledgerCache")
	}

	if p.cache.Size() > maxBatchSize {
		err = p.Commit(ctx)
		if err != nil {
			return errors.Wrap(err, "error in Commit")
		}
	}

	return nil
}

func (p *ClaimableBalancesChangeProcessor) Commit(ctx context.Context) error {
	defer p.reset()
	var (
		cbIDsToDelete []string
	)
	changes := p.cache.GetChanges()
	for _, change := range changes {
		switch {
		case change.Pre == nil && change.Post != nil:
			// Created
			cb, err := p.ledgerEntryToRow(change.Post)
			if err != nil {
				return err
			}
			// Add claimable balance
			if err := p.claimableBalanceInsertBuilder.Add(cb); err != nil {
				return errors.Wrap(err, "error adding to ClaimableBalanceBatchInsertBuilder")
			}

			// Add claimants
			for _, claimant := range cb.Claimants {
				claimant := history.ClaimableBalanceClaimant{
					BalanceID:          cb.BalanceID,
					Destination:        claimant.Destination,
					LastModifiedLedger: cb.LastModifiedLedger,
				}

				if err := p.claimantsInsertBuilder.Add(claimant); err != nil {
					return errors.Wrap(err, "error adding to ClaimableBalanceClaimantBatchInsertBuilder")
				}
			}
		case change.Pre != nil && change.Post == nil:
			// Removed
			cBalance := change.Pre.Data.MustClaimableBalance()
			id, err := p.encodingBuffer.MarshalHex(cBalance.BalanceId)
			if err != nil {
				return err
			}
			cbIDsToDelete = append(cbIDsToDelete, id)
		default:
			// claimable balance can only be created or removed
			return fmt.Errorf("invalid change entry for a claimable balance was detected")
		}
	}

	err := p.claimantsInsertBuilder.Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "error executing ClaimableBalanceClaimantBatchInsertBuilder")
	}

	err = p.claimableBalanceInsertBuilder.Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "error executing ClaimableBalanceBatchInsertBuilder")
	}

	if len(cbIDsToDelete) > 0 {
		count, err := p.qClaimableBalances.RemoveClaimableBalances(ctx, cbIDsToDelete)
		if err != nil {
			return errors.Wrap(err, "error executing removal")
		}
		if count != int64(len(cbIDsToDelete)) {
			return ingest.NewStateError(errors.Errorf(
				"%d rows affected when deleting %d claimable balances",
				count,
				len(cbIDsToDelete),
			))
		}

		// Remove ClaimableBalanceClaimants
		_, err = p.qClaimableBalances.RemoveClaimableBalanceClaimants(ctx, cbIDsToDelete)
		if err != nil {
			return errors.Wrap(err, "error executing removal of claimants")
		}
	}

	return nil
}

func buildClaimants(claimants []xdr.Claimant) history.Claimants {
	hClaimants := history.Claimants{}
	for _, c := range claimants {
		xc := c.MustV0()
		hClaimants = append(hClaimants, history.Claimant{
			Destination: xc.Destination.Address(),
			Predicate:   xc.Predicate,
		})
	}
	return hClaimants
}

func (p *ClaimableBalancesChangeProcessor) ledgerEntryToRow(entry *xdr.LedgerEntry) (history.ClaimableBalance, error) {
	cBalance := entry.Data.MustClaimableBalance()
	id, err := xdr.MarshalHex(cBalance.BalanceId)
	if err != nil {
		return history.ClaimableBalance{}, err
	}
	row := history.ClaimableBalance{
		BalanceID:          id,
		Claimants:          buildClaimants(cBalance.Claimants),
		Asset:              cBalance.Asset,
		Amount:             cBalance.Amount,
		Sponsor:            ledgerEntrySponsorToNullString(*entry),
		LastModifiedLedger: uint32(entry.LastModifiedLedgerSeq),
		Flags:              uint32(cBalance.Flags()),
	}
	return row, nil
}
