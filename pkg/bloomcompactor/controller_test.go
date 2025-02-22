package bloomcompactor

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	v1 "github.com/grafana/loki/pkg/storage/bloom/v1"
	"github.com/grafana/loki/pkg/storage/stores/shipper/indexshipper/tsdb"
)

func Test_findGaps(t *testing.T) {
	for _, tc := range []struct {
		desc           string
		err            bool
		exp            []v1.FingerprintBounds
		ownershipRange v1.FingerprintBounds
		metas          []v1.FingerprintBounds
	}{
		{
			desc:           "error nonoverlapping metas",
			err:            true,
			exp:            nil,
			ownershipRange: v1.NewBounds(0, 10),
			metas:          []v1.FingerprintBounds{v1.NewBounds(11, 20)},
		},
		{
			desc:           "one meta with entire ownership range",
			err:            false,
			exp:            nil,
			ownershipRange: v1.NewBounds(0, 10),
			metas:          []v1.FingerprintBounds{v1.NewBounds(0, 10)},
		},
		{
			desc:           "two non-overlapping metas with entire ownership range",
			err:            false,
			exp:            nil,
			ownershipRange: v1.NewBounds(0, 10),
			metas: []v1.FingerprintBounds{
				v1.NewBounds(0, 5),
				v1.NewBounds(6, 10),
			},
		},
		{
			desc:           "two overlapping metas with entire ownership range",
			err:            false,
			exp:            nil,
			ownershipRange: v1.NewBounds(0, 10),
			metas: []v1.FingerprintBounds{
				v1.NewBounds(0, 6),
				v1.NewBounds(4, 10),
			},
		},
		{
			desc: "one meta with partial ownership range",
			err:  false,
			exp: []v1.FingerprintBounds{
				v1.NewBounds(6, 10),
			},
			ownershipRange: v1.NewBounds(0, 10),
			metas: []v1.FingerprintBounds{
				v1.NewBounds(0, 5),
			},
		},
		{
			desc: "smaller subsequent meta with partial ownership range",
			err:  false,
			exp: []v1.FingerprintBounds{
				v1.NewBounds(8, 10),
			},
			ownershipRange: v1.NewBounds(0, 10),
			metas: []v1.FingerprintBounds{
				v1.NewBounds(0, 7),
				v1.NewBounds(3, 4),
			},
		},
		{
			desc: "hole in the middle",
			err:  false,
			exp: []v1.FingerprintBounds{
				v1.NewBounds(4, 5),
			},
			ownershipRange: v1.NewBounds(0, 10),
			metas: []v1.FingerprintBounds{
				v1.NewBounds(0, 3),
				v1.NewBounds(6, 10),
			},
		},
		{
			desc: "holes on either end",
			err:  false,
			exp: []v1.FingerprintBounds{
				v1.NewBounds(0, 2),
				v1.NewBounds(8, 10),
			},
			ownershipRange: v1.NewBounds(0, 10),
			metas: []v1.FingerprintBounds{
				v1.NewBounds(3, 5),
				v1.NewBounds(6, 7),
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gaps, err := findGaps(tc.ownershipRange, tc.metas)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.Equal(t, tc.exp, gaps)
		})
	}
}

func tsdbID(n int) tsdb.SingleTenantTSDBIdentifier {
	return tsdb.SingleTenantTSDBIdentifier{
		TS: time.Unix(int64(n), 0),
	}
}

func genMeta(min, max model.Fingerprint, sources []int, blocks []BlockRef) Meta {
	m := Meta{
		OwnershipRange: v1.NewBounds(min, max),
		Blocks:         blocks,
	}
	for _, source := range sources {
		m.Sources = append(m.Sources, tsdbID(source))
	}
	return m
}

func Test_gapsBetweenTSDBsAndMetas(t *testing.T) {

	for _, tc := range []struct {
		desc           string
		err            bool
		exp            []tsdbGaps
		ownershipRange v1.FingerprintBounds
		tsdbs          []tsdb.Identifier
		metas          []Meta
	}{
		{
			desc:           "non-overlapping tsdbs and metas",
			err:            true,
			ownershipRange: v1.NewBounds(0, 10),
			tsdbs:          []tsdb.Identifier{tsdbID(0)},
			metas: []Meta{
				genMeta(11, 20, []int{0}, nil),
			},
		},
		{
			desc:           "single tsdb",
			ownershipRange: v1.NewBounds(0, 10),
			tsdbs:          []tsdb.Identifier{tsdbID(0)},
			metas: []Meta{
				genMeta(4, 8, []int{0}, nil),
			},
			exp: []tsdbGaps{
				{
					tsdb: tsdbID(0),
					gaps: []v1.FingerprintBounds{
						v1.NewBounds(0, 3),
						v1.NewBounds(9, 10),
					},
				},
			},
		},
		{
			desc:           "multiple tsdbs with separate blocks",
			ownershipRange: v1.NewBounds(0, 10),
			tsdbs:          []tsdb.Identifier{tsdbID(0), tsdbID(1)},
			metas: []Meta{
				genMeta(0, 5, []int{0}, nil),
				genMeta(6, 10, []int{1}, nil),
			},
			exp: []tsdbGaps{
				{
					tsdb: tsdbID(0),
					gaps: []v1.FingerprintBounds{
						v1.NewBounds(6, 10),
					},
				},
				{
					tsdb: tsdbID(1),
					gaps: []v1.FingerprintBounds{
						v1.NewBounds(0, 5),
					},
				},
			},
		},
		{
			desc:           "multiple tsdbs with the same blocks",
			ownershipRange: v1.NewBounds(0, 10),
			tsdbs:          []tsdb.Identifier{tsdbID(0), tsdbID(1)},
			metas: []Meta{
				genMeta(0, 5, []int{0, 1}, nil),
				genMeta(6, 8, []int{1}, nil),
			},
			exp: []tsdbGaps{
				{
					tsdb: tsdbID(0),
					gaps: []v1.FingerprintBounds{
						v1.NewBounds(6, 10),
					},
				},
				{
					tsdb: tsdbID(1),
					gaps: []v1.FingerprintBounds{
						v1.NewBounds(9, 10),
					},
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gaps, err := gapsBetweenTSDBsAndMetas(tc.ownershipRange, tc.tsdbs, tc.metas)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.Equal(t, tc.exp, gaps)
		})
	}
}

func genBlockRef(min, max model.Fingerprint) BlockRef {
	bounds := v1.NewBounds(min, max)
	return BlockRef{
		OwnershipRange: bounds,
	}
}

func Test_blockPlansForGaps(t *testing.T) {
	for _, tc := range []struct {
		desc           string
		ownershipRange v1.FingerprintBounds
		tsdbs          []tsdb.Identifier
		metas          []Meta
		err            bool
		exp            []blockPlan
	}{
		{
			desc:           "single overlapping meta+no overlapping block",
			ownershipRange: v1.NewBounds(0, 10),
			tsdbs:          []tsdb.Identifier{tsdbID(0)},
			metas: []Meta{
				genMeta(5, 20, []int{1}, []BlockRef{genBlockRef(11, 20)}),
			},
			exp: []blockPlan{
				{
					tsdb: tsdbID(0),
					gaps: []gapWithBlocks{
						{
							bounds: v1.NewBounds(0, 10),
						},
					},
				},
			},
		},
		{
			desc:           "single overlapping meta+one overlapping block",
			ownershipRange: v1.NewBounds(0, 10),
			tsdbs:          []tsdb.Identifier{tsdbID(0)},
			metas: []Meta{
				genMeta(5, 20, []int{1}, []BlockRef{genBlockRef(9, 20)}),
			},
			exp: []blockPlan{
				{
					tsdb: tsdbID(0),
					gaps: []gapWithBlocks{
						{
							bounds: v1.NewBounds(0, 10),
							blocks: []BlockRef{genBlockRef(9, 20)},
						},
					},
				},
			},
		},
		{
			// the range which needs to be generated doesn't overlap with existing blocks
			// from other tsdb versions since theres an up to date tsdb version block,
			// but we can trim the range needing generation
			desc:           "trims up to date area",
			ownershipRange: v1.NewBounds(0, 10),
			tsdbs:          []tsdb.Identifier{tsdbID(0)},
			metas: []Meta{
				genMeta(9, 20, []int{0}, []BlockRef{genBlockRef(9, 20)}), // block for same tsdb
				genMeta(9, 20, []int{1}, []BlockRef{genBlockRef(9, 20)}), // block for different tsdb
			},
			exp: []blockPlan{
				{
					tsdb: tsdbID(0),
					gaps: []gapWithBlocks{
						{
							bounds: v1.NewBounds(0, 8),
						},
					},
				},
			},
		},
		{
			desc:           "uses old block for overlapping range",
			ownershipRange: v1.NewBounds(0, 10),
			tsdbs:          []tsdb.Identifier{tsdbID(0)},
			metas: []Meta{
				genMeta(9, 20, []int{0}, []BlockRef{genBlockRef(9, 20)}), // block for same tsdb
				genMeta(5, 20, []int{1}, []BlockRef{genBlockRef(5, 20)}), // block for different tsdb
			},
			exp: []blockPlan{
				{
					tsdb: tsdbID(0),
					gaps: []gapWithBlocks{
						{
							bounds: v1.NewBounds(0, 8),
							blocks: []BlockRef{genBlockRef(5, 20)},
						},
					},
				},
			},
		},
		{
			desc:           "multi case",
			ownershipRange: v1.NewBounds(0, 10),
			tsdbs:          []tsdb.Identifier{tsdbID(0), tsdbID(1)}, // generate for both tsdbs
			metas: []Meta{
				genMeta(0, 2, []int{0}, []BlockRef{
					genBlockRef(0, 1),
					genBlockRef(1, 2),
				}), // tsdb_0
				genMeta(6, 8, []int{0}, []BlockRef{genBlockRef(6, 8)}), // tsdb_0

				genMeta(3, 5, []int{1}, []BlockRef{genBlockRef(3, 5)}),   // tsdb_1
				genMeta(8, 10, []int{1}, []BlockRef{genBlockRef(8, 10)}), // tsdb_1
			},
			exp: []blockPlan{
				{
					tsdb: tsdbID(0),
					gaps: []gapWithBlocks{
						// tsdb (id=0) can source chunks from the blocks built from tsdb (id=1)
						{
							bounds: v1.NewBounds(3, 5),
							blocks: []BlockRef{genBlockRef(3, 5)},
						},
						{
							bounds: v1.NewBounds(9, 10),
							blocks: []BlockRef{genBlockRef(8, 10)},
						},
					},
				},
				// tsdb (id=1) can source chunks from the blocks built from tsdb (id=0)
				{
					tsdb: tsdbID(1),
					gaps: []gapWithBlocks{
						{
							bounds: v1.NewBounds(0, 2),
							blocks: []BlockRef{
								genBlockRef(0, 1),
								genBlockRef(1, 2),
							},
						},
						{
							bounds: v1.NewBounds(6, 7),
							blocks: []BlockRef{genBlockRef(6, 8)},
						},
					},
				},
			},
		},
		{
			desc:           "dedupes block refs",
			ownershipRange: v1.NewBounds(0, 10),
			tsdbs:          []tsdb.Identifier{tsdbID(0)},
			metas: []Meta{
				genMeta(9, 20, []int{1}, []BlockRef{
					genBlockRef(1, 4),
					genBlockRef(9, 20),
				}), // blocks for first diff tsdb
				genMeta(5, 20, []int{2}, []BlockRef{
					genBlockRef(5, 10),
					genBlockRef(9, 20), // same block references in prior meta (will be deduped)
				}), // block for second diff tsdb
			},
			exp: []blockPlan{
				{
					tsdb: tsdbID(0),
					gaps: []gapWithBlocks{
						{
							bounds: v1.NewBounds(0, 10),
							blocks: []BlockRef{
								genBlockRef(1, 4),
								genBlockRef(5, 10),
								genBlockRef(9, 20),
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			// we reuse the gapsBetweenTSDBsAndMetas function to generate the gaps as this function is tested
			// separately and it's used to generate input in our regular code path (easier to write tests this way).
			gaps, err := gapsBetweenTSDBsAndMetas(tc.ownershipRange, tc.tsdbs, tc.metas)
			require.NoError(t, err)

			plans, err := blockPlansForGaps(gaps, tc.metas)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.Equal(t, tc.exp, plans)

		})
	}
}
