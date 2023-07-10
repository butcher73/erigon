package state

import (
	"context"
	"encoding/binary"
	"math"
	"sync/atomic"
	"testing"

	"github.com/ledgerwatch/log/v3"
	"github.com/stretchr/testify/require"
)

func BenchmarkName2(b *testing.B) {
	b.Run("1", func(b *testing.B) {
		j := atomic.Int32{}
		for i := 0; i < b.N; i++ {
			j.Add(1)
		}
	})
	b.Run("2", func(b *testing.B) {
		j := &atomic.Int32{}
		for i := 0; i < b.N; i++ {
			j.Add(1)
		}
	})
}

func TestLocality(t *testing.T) {
	logger := log.New()
	ctx, require := context.Background(), require.New(t)
	const Module uint64 = 31
	_, db, ii, txs := filledInvIndexOfSize(t, 300, 4, Module, logger)
	mergeInverted(t, db, ii, txs)

	{ //prepare
		ii.withLocalityIndex = true
		var err error
		ii.localityIndex, err = NewLocalityIndex(ii.dir, ii.tmpdir, ii.aggregationStep, ii.filenameBase, ii.logger)
		require.NoError(err)

		ic := ii.MakeContext()
		defer ic.Close()
		err = ic.BuildOptionalMissedIndices(ctx)
		require.NoError(err)
		ic.Close()
	}

	t.Run("locality iterator", func(t *testing.T) {
		ic := ii.MakeContext()
		defer ic.Close()
		it := ic.iterateKeysLocality(math.MaxUint64)
		require.True(it.HasNext())
		key, bitmap := it.Next()
		require.Equal(uint64(1), binary.BigEndian.Uint64(key))
		require.Equal([]uint64{0, 1}, bitmap)
		require.True(it.HasNext())
		key, bitmap = it.Next()
		require.Equal(uint64(2), binary.BigEndian.Uint64(key))
		require.Equal([]uint64{0, 1}, bitmap)

		var last []byte
		for it.HasNext() {
			key, _ = it.Next()
			last = key
		}
		require.Equal(Module-1, binary.BigEndian.Uint64(last))
	})

	t.Run("locality index: getBeforeTxNum full bitamp", func(t *testing.T) {
		ic := ii.MakeContext()
		defer ic.Close()

		res, err := ic.loc.bm.At(0)
		require.NoError(err)
		require.Equal([]uint64{0, 1}, res)
		res, err = ic.loc.bm.At(1)
		require.NoError(err)
		require.Equal([]uint64{0, 1}, res)
		res, err = ic.loc.bm.At(32) //too big, must error
		require.Error(err)
		require.Empty(res)
	})

	t.Run("locality index: search from given position", func(t *testing.T) {
		ic := ii.MakeContext()
		defer ic.Close()
		fst, snd, ok1, ok2, err := ic.loc.bm.First2At(0, 1)
		require.NoError(err)
		require.True(ok1)
		require.False(ok2)
		require.Equal(uint64(1), fst)
		require.Zero(snd)
	})
	t.Run("locality index: search from given position in future", func(t *testing.T) {
		ic := ii.MakeContext()
		defer ic.Close()
		fst, snd, ok1, ok2, err := ic.loc.bm.First2At(0, 2)
		require.NoError(err)
		require.False(ok1)
		require.False(ok2)
		require.Zero(fst)
		require.Zero(snd)
	})
	t.Run("locality index: lookup", func(t *testing.T) {
		ic := ii.MakeContext()
		defer ic.Close()

		var k [8]byte
		binary.BigEndian.PutUint64(k[:], 1)
		v1, v2, from, ok1, ok2 := ic.ii.localityIndex.lookupIdxFiles(ic.loc, k[:], 1*ic.ii.aggregationStep*StepsInBiggestFile)
		require.True(ok1)
		require.False(ok2)
		require.Equal(uint64(1*StepsInBiggestFile), v1)
		require.Equal(uint64(0*StepsInBiggestFile), v2)
		require.Equal(2*ic.ii.aggregationStep*StepsInBiggestFile, from)
	})
}

func TestLocalityDomain(t *testing.T) {
	logger := log.New()
	ctx, require := context.Background(), require.New(t)
	keyCount, txCount := uint64(200), uint64(300)
	db, dom, data := filledDomainFixedSize(t, keyCount, txCount, 4, logger)
	collateAndMerge(t, db, nil, dom, txCount)

	{ //prepare
		dom.withLocalityIndex = true
		var err error
		dom.domainLocalityIndex, err = NewLocalityIndex(dom.dir, dom.tmpdir, dom.aggregationStep, dom.filenameBase+"_kv", dom.logger)
		require.NoError(err)

		dc := dom.MakeContext()
		defer dom.Close()
		err = dc.BuildOptionalMissedIndices(ctx)
		require.NoError(err)
		dc.Close()
	}

	_, _ = ctx, data
	t.Run("locality iterator", func(t *testing.T) {
		ic := dom.MakeContext()
		defer dom.Close()
		it := ic.iterateKeysLocality(math.MaxUint64)
		require.True(it.HasNext())
		key, bitmap := it.Next()
		require.Equal(uint64(1), binary.BigEndian.Uint64(key))
		require.Equal([]uint64{0, 1}, bitmap)
		require.True(it.HasNext())
		key, bitmap = it.Next()
		require.Equal(uint64(2), binary.BigEndian.Uint64(key))
		require.Equal([]uint64{0, 1}, bitmap)

		var last []byte
		for it.HasNext() {
			key, _ := it.Next()
			last = key
		}
		require.Equal(int(keyCount-1), int(binary.BigEndian.Uint64(last)))
	})

	t.Run("locality index: getBeforeTxNum full bitamp", func(t *testing.T) {
		dc := dom.MakeContext()
		defer dc.Close()
		res, err := dc.loc.bm.At(0)
		require.NoError(err)
		require.Equal([]uint64{0, 1}, res)
		res, err = dc.loc.bm.At(1)
		require.NoError(err)
		require.Equal([]uint64{0, 1}, res)
		res, err = dc.loc.bm.At(keyCount) //too big, must error
		require.Error(err)
		require.Empty(res)
	})

	t.Run("locality index: search from given position", func(t *testing.T) {
		dc := dom.MakeContext()
		defer dc.Close()
		fst, snd, ok1, ok2, err := dc.loc.bm.First2At(0, 1)
		require.NoError(err)
		require.True(ok1)
		require.False(ok2)
		require.Equal(uint64(1), fst)
		require.Zero(snd)
	})
	t.Run("locality index: search from given position in future", func(t *testing.T) {
		dc := dom.MakeContext()
		defer dc.Close()
		fst, snd, ok1, ok2, err := dc.loc.bm.First2At(0, 2)
		require.NoError(err)
		require.False(ok1)
		require.False(ok2)
		require.Zero(fst)
		require.Zero(snd)
	})
	t.Run("locality index: lookup", func(t *testing.T) {
		dc := dom.MakeContext()
		defer dc.Close()
		var k [8]byte
		binary.BigEndian.PutUint64(k[:], 1)
		v1, v2, from, ok1, ok2 := dc.d.localityIndex.lookupIdxFiles(dc.loc, k[:], 1*dc.d.aggregationStep*StepsInBiggestFile)
		require.True(ok1)
		require.False(ok2)
		require.Equal(uint64(1*StepsInBiggestFile), v1)
		require.Equal(uint64(0*StepsInBiggestFile), v2)
		require.Equal(2*dc.d.aggregationStep*StepsInBiggestFile, from)
	})
}
