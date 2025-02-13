package history

import (
	"encoding/json"
	"testing"

	"github.com/guregu/null"

	"github.com/stellar/go/services/horizon/internal/test"
	"github.com/stellar/go/toid"
)

func TestAddEffect(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &Q{tt.HorizonSession()}
	tt.Assert.NoError(q.Begin(tt.Ctx))

	address := "GAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSTVY"
	muxedAddres := "MAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSAAAAAAAAAAE2LP26"
	accountLoader := NewAccountLoader()

	builder := q.NewEffectBatchInsertBuilder()
	sequence := int32(56)
	details, err := json.Marshal(map[string]string{
		"amount":     "1000.0000000",
		"asset_type": "native",
	})

	err = builder.Add(
		accountLoader.GetFuture(address),
		null.StringFrom(muxedAddres),
		toid.New(sequence, 1, 1).ToInt64(),
		1,
		3,
		details,
	)
	tt.Assert.NoError(err)

	tt.Assert.NoError(accountLoader.Exec(tt.Ctx, q))
	tt.Assert.NoError(builder.Exec(tt.Ctx, q))
	tt.Assert.NoError(q.Commit())

	effects := []Effect{}
	tt.Assert.NoError(q.Effects().Select(tt.Ctx, &effects))
	tt.Assert.Len(effects, 1)

	effect := effects[0]
	tt.Assert.Equal(address, effect.Account)
	tt.Assert.Equal(muxedAddres, effect.AccountMuxed.String)
	tt.Assert.Equal(int64(240518172673), effect.HistoryOperationID)
	tt.Assert.Equal(int32(1), effect.Order)
	tt.Assert.Equal(EffectType(3), effect.Type)
	tt.Assert.Equal("{\"amount\": \"1000.0000000\", \"asset_type\": \"native\"}", effect.DetailsString.String)
}
