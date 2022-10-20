// Copyright 2022 bnb-chain. All Rights Reserved.
//
// Distributed under MIT license.
// See file LICENSE for detail or copy at https://opensource.org/licenses/MIT

package bsmt

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"

	"github.com/bnb-chain/zkbnb-smt/database"
	wrappedLevelDB "github.com/bnb-chain/zkbnb-smt/database/leveldb"
	"github.com/bnb-chain/zkbnb-smt/database/memory"
	wrappedRedis "github.com/bnb-chain/zkbnb-smt/database/redis"
)

var (
	nilHash = common.FromHex("01ef55cdf3b9b0d65e6fb6317f79627534d971fd96c811281af618c0028d5e7a")
)

type testEnv struct {
	tag    string
	hasher *Hasher
	db     database.TreeDB
}

func prepareEnv(t *testing.T) []testEnv {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	pipe := client.Pipeline()
	return []testEnv{
		{
			tag:    "memoryDB",
			hasher: &Hasher{sha256.New()},
			db:     memory.NewMemoryDB(),
		},
		{
			tag:    "levelDB",
			hasher: &Hasher{sha256.New()},
			db:     wrappedLevelDB.WrapWithNamespace(wrappedLevelDB.NewFromExistLevelDB(db), "test"),
		},
		{
			tag:    "redis",
			hasher: &Hasher{sha256.New()},
			db:     wrappedRedis.WrapWithNamespace(wrappedRedis.NewFromExistRedisClient(client, wrappedRedis.WithSharedPipeliner(pipe)), "test"),
		},
	}
}

func testProof(t *testing.T, hasher *Hasher, db database.TreeDB) {
	smt, err := NewBASSparseMerkleTree(hasher, db, 8, nilHash)
	if err != nil {
		t.Fatal(err)
	}

	emptyProof, err := smt.GetProof(255)
	if err != nil {
		t.Fatal(err)
	}
	if !smt.VerifyProof(0, emptyProof) {
		t.Fatal("verify empty proof failed")
	}

	key1 := uint64(0)
	key2 := uint64(255)
	key3 := uint64(213)
	val1 := hasher.Hash([]byte("test1"))
	version := smt.LatestVersion()
	_, err = smt.Get(key1, &version)
	if err == nil {
		t.Fatal("tree contains element before write")
	} else if !errors.Is(err, ErrEmptyRoot) {
		t.Fatal(err)
	}

	val2 := hasher.Hash([]byte("test2"))
	val3 := hasher.Hash([]byte("test3"))
	smt.Set(key1, val1)
	version1, err := smt.Commit(nil)
	if err != nil {
		t.Fatal(err)
	}
	smt.Set(key2, val2)
	version, err = smt.Commit(nil)
	if err != nil {
		t.Fatal(err)
	}
	smt.Set(key3, val3)
	version, err = smt.Commit(&version1)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		// test commit nil data
		version, err = smt.Commit(&version)
		if err != nil {
			t.Fatal(err)
		}
	}

	emptyProof, err = smt.GetProof(44)
	if err != nil {
		t.Fatal(err)
	}
	if !smt.VerifyProof(44, emptyProof) {
		t.Fatal("verify empty proof failed")
	}

	hash1, err := smt.Get(key1, &version)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(hash1, val1) {
		t.Fatalf("not equal to the original, want: %v, got: %v", val1, hash1)
	}

	hash2, err := smt.Get(key2, &version)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(hash2, val2) {
		t.Fatalf("not equal to the original, want: %v, got: %v", val2, hash2)
	}

	hash3, err := smt.Get(key3, &version)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(hash3, val3) {
		t.Fatalf("not equal to the original, want: %v, got: %v", val3, hash3)
	}

	proof, err := smt.GetProof(key1)
	if err != nil {
		t.Fatal(err)
	}

	if !smt.VerifyProof(key1, proof) {
		t.Fatal("verify proof1 failed")
	}

	proof, err = smt.GetProof(key2)
	if err != nil {
		t.Fatal(err)
	}

	if !smt.VerifyProof(key2, proof) {
		t.Fatal("verify proof2 failed")
	}

	proof, err = smt.GetProof(key3)
	if err != nil {
		t.Fatal(err)
	}

	if !smt.VerifyProof(key3, proof) {
		t.Fatal("verify proof3 failed")
	}

	// restore tree from db
	smt2, err := NewBASSparseMerkleTree(hasher, db, 8, nilHash)
	if err != nil {
		t.Fatal(err)
	}

	hash11, err := smt2.Get(key1, &version)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(hash1, hash11) {
		t.Fatalf("not equal to the original, want: %v, got: %v", hash1, hash11)
	}

	hash22, err := smt2.Get(key2, &version)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(hash2, hash22) {
		t.Fatalf("not equal to the original, want: %v, got: %v", hash2, hash22)
	}

	hash33, err := smt2.Get(key3, &version)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(hash3, hash33) {
		t.Fatalf("not equal to the original, want: %v, got: %v", hash3, hash33)
	}

	proof, err = smt2.GetProof(key1)
	if err != nil {
		t.Fatal(err)
	}

	if !smt.VerifyProof(key1, proof) {
		t.Fatal("verify proof1 failed")
	}

	proof, err = smt2.GetProof(key2)
	if err != nil {
		t.Fatal(err)
	}

	if !smt.VerifyProof(key2, proof) {
		t.Fatal("verify proof2 failed")
	}

	proof, err = smt2.GetProof(key3)
	if err != nil {
		t.Fatal(err)
	}

	if !smt.VerifyProof(key3, proof) {
		t.Fatal("verify proof3 failed")
	}

	key4 := uint64(1)
	val4 := hasher.Hash([]byte("test4"))
	err = smt2.Set(key4, val4)
	if err != nil {
		t.Fatal(err)
	}
	_, err = smt2.Commit(nil)
	if err != nil {
		t.Fatal(err)
	}

	proof, err = smt2.GetProof(key4)
	if err != nil {
		t.Fatal(err)
	}

	if !smt2.VerifyProof(key4, proof) {
		t.Fatal("verify proof4 failed")
	}
}

func Test_BASSparseMerkleTree_Proof(t *testing.T) {
	for _, env := range prepareEnv(t) {
		t.Logf("test [%s]", env.tag)
		testProof(t, env.hasher, env.db)
		env.db.Close()
	}
}

func testRollback(t *testing.T, hasher *Hasher, db database.TreeDB) {
	smt, err := NewBASSparseMerkleTree(hasher, db, 8, nilHash)
	if err != nil {
		t.Fatal(err)
	}

	key1 := uint64(1)
	key2 := uint64(2)
	key3 := uint64(23)
	val1 := hasher.Hash([]byte("test1"))
	val2 := hasher.Hash([]byte("test2"))
	val3 := hasher.Hash([]byte("test3"))
	smt.Set(key1, val1)
	smt.Set(key2, val2)

	version1, err := smt.Commit(nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = smt.Get(key1, &version1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = smt.Get(key2, &version1)
	if err != nil {
		t.Fatal(err)
	}

	proof2, err := smt.GetProof(key2)
	if err != nil {
		t.Fatal(err)
	}
	if !smt.VerifyProof(key2, proof2) {
		t.Fatal("verify proof2 failed")
	}

	smt.Set(key3, val3)
	version2, err := smt.Commit(nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = smt.Get(key3, &version2)
	if err != nil {
		t.Fatal(err)
	}

	err = smt.Rollback(version1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = smt.Get(key3, &version2)
	if !errors.Is(err, ErrVersionTooHigh) {
		t.Fatal(err)
	}

	if !smt.VerifyProof(key2, proof2) {
		t.Fatal("verify proof2 after rollback failed")
	}

	// restore tree from db
	smt2, err := NewBASSparseMerkleTree(hasher, db, 8, nilHash)
	if err != nil {
		t.Fatal(err)
	}
	_, err = smt2.Get(key3, &version2)
	if !errors.Is(err, ErrVersionTooHigh) {
		t.Fatal(err)
	}

	if !smt2.VerifyProof(key2, proof2) {
		t.Fatal("verify proof2 after restoring from db failed")
	}
}

func Test_BASSparseMerkleTree_Rollback(t *testing.T) {
	for _, env := range prepareEnv(t) {
		t.Logf("test [%s]", env.tag)
		testRollback(t, env.hasher, env.db)
		env.db.Close()
	}
}

func testRollbackRecovery(t *testing.T, hasher *Hasher, db database.TreeDB) {
	db1 := memory.NewMemoryDB()
	db2 := memory.NewMemoryDB()
	smt, err := NewBASSparseMerkleTree(hasher, db1, 8, nilHash)
	if err != nil {
		t.Fatal(err)
	}
	smt2, err := NewBASSparseMerkleTree(hasher, db2, 8, nilHash)
	if err != nil {
		t.Fatal(err)
	}

	key1 := uint64(0)
	key2 := uint64(1)
	val1 := hasher.Hash([]byte("test1"))
	val2 := hasher.Hash([]byte("test2"))
	val3 := hasher.Hash([]byte("test3"))
	val4 := hasher.Hash([]byte("test4"))
	smt.Set(key1, val1)
	smt.Set(key2, val2)
	smt2.Set(key1, val1)
	smt2.Set(key2, val2)
	version1, err := smt.Commit(nil)
	if err != nil {
		t.Fatal(err)
	}
	version1, err = smt2.Commit(nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = smt.Get(key1, &version1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = smt.Get(key2, &version1)
	if err != nil {
		t.Fatal(err)
	}

	proof2, err := smt.GetProof(key2)
	if err != nil {
		t.Fatal(err)
	}
	if !smt.VerifyProof(key2, proof2) {
		t.Fatal("verify proof2 failed")
	}

	smt.Set(key1, val3)
	smt.Set(key2, val4)
	smt2.Set(key1, val3)
	smt2.Set(key2, val4)
	_, err = smt.Commit(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = smt2.Commit(nil)
	if err != nil {
		t.Fatal(err)
	}

	// restore tree from db
	smt2, err = NewBASSparseMerkleTree(hasher, db2, 8, nilHash)
	if err != nil {
		t.Fatal(err)
	}

	err = smt2.Rollback(version1)
	if err != nil {
		t.Fatal(err)
	}

	err = smt.Rollback(version1)
	if err != nil {
		t.Fatal(err)
	}

	proof2, err = smt2.GetProof(key2)
	if err != nil {
		t.Fatal(err)
	}
	if !smt.VerifyProof(key2, proof2) {
		t.Fatal("[origin] verify proof2 failed")
	}

	proof2, err = smt.GetProof(key2)
	if err != nil {
		t.Fatal(err)
	}
	if !smt2.VerifyProof(key2, proof2) {
		t.Fatal("[recovery] verify proof2 failed")
	}
}

func Test_BASSparseMerkleTree_RollbackAfterRecovery(t *testing.T) {
	for _, env := range prepareEnv(t) {
		t.Logf("test [%s]", env.tag)
		testRollbackRecovery(t, env.hasher, env.db)
		env.db.Close()
	}
}

func testReset(t *testing.T, hasher *Hasher, db database.TreeDB) {
	smt, err := NewBASSparseMerkleTree(hasher, db, 8, nilHash)
	if err != nil {
		t.Fatal(err)
	}

	key1 := uint64(1)
	key2 := uint64(2)
	key3 := uint64(3)
	val1 := hasher.Hash([]byte("test1"))
	val2 := hasher.Hash([]byte("test2"))
	val3 := hasher.Hash([]byte("test3"))
	smt.Set(key1, val1)
	smt.Set(key2, val2)

	version1, err := smt.Commit(nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = smt.Get(key1, &version1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = smt.Get(key2, &version1)
	if err != nil {
		t.Fatal(err)
	}

	smt.Set(key3, val3)
	smt.Reset()
}

func Test_BASSparseMerkleTree_Reset(t *testing.T) {
	for _, env := range prepareEnv(t) {
		t.Logf("test [%s]", env.tag)
		testReset(t, env.hasher, env.db)
		env.db.Close()
	}
}

func testGC(t *testing.T, hasher *Hasher, db database.TreeDB) {
	smt, err := NewBASSparseMerkleTree(hasher, db, 8, nilHash,
		GCThreshold(1024*10))
	if err != nil {
		t.Fatal(err)
	}

	testKVData := []struct {
		key uint64
		val []byte
	}{
		{1, hasher.Hash([]byte("val1"))},
		{2, hasher.Hash([]byte("val2"))},
		{3, hasher.Hash([]byte("val3"))},
		{4, hasher.Hash([]byte("val4"))},
		{5, hasher.Hash([]byte("val5"))},
		{6, hasher.Hash([]byte("val6"))},
		{7, hasher.Hash([]byte("val7"))},
		{8, hasher.Hash([]byte("val8"))},
		{9, hasher.Hash([]byte("val9"))},
		{10, hasher.Hash([]byte("val10"))},
		{11, hasher.Hash([]byte("val11"))},
		{12, hasher.Hash([]byte("val12"))},
		{13, hasher.Hash([]byte("val13"))},
		{14, hasher.Hash([]byte("val14"))},
		{200, hasher.Hash([]byte("val200"))},
		{20, hasher.Hash([]byte("val20"))},
		{21, hasher.Hash([]byte("val21"))},
		{22, hasher.Hash([]byte("val22"))},
		{23, hasher.Hash([]byte("val23"))},
		{24, hasher.Hash([]byte("val24"))},
		{26, hasher.Hash([]byte("val26"))},
		{37, hasher.Hash([]byte("val37"))},
		{255, hasher.Hash([]byte("val255"))},
		{254, hasher.Hash([]byte("val254"))},
		{253, hasher.Hash([]byte("val253"))},
		{252, hasher.Hash([]byte("val252"))},
		{251, hasher.Hash([]byte("val251"))},
		{250, hasher.Hash([]byte("val250"))},
		{249, hasher.Hash([]byte("val249"))},
		{248, hasher.Hash([]byte("val248"))},
		{247, hasher.Hash([]byte("val247"))},
		{15, hasher.Hash([]byte("val15"))},
	}

	t.Log("set data")
	for version, testData := range testKVData {
		smt.Set(testData.key, testData.val)
		if version >= 2 {
			pruneVer := Version(version - 1)
			_, err = smt.Commit(&pruneVer)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			_, err = smt.Commit(nil)
			if err != nil {
				t.Fatal(err)
			}
		}

		t.Log("tree.Size() = ", smt.Size())
	}

	t.Log("verify proofs")
	for _, testData := range testKVData {
		proof, err := smt.GetProof(testData.key)
		if err != nil {
			t.Fatal(err)
		}
		if !smt.VerifyProof(testData.key, proof) {
			t.Fatalf("verify proof of key [%d] failed", testData.key)
		}
		t.Log("tree.Size() = ", smt.Size())
	}

	t.Log("test gc")
	smt.Set(0, hasher.Hash([]byte("val0")))
	pruneVer := Version(len(testKVData) - 2)
	_, err = smt.Commit(&pruneVer)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("tree.Size() = ", smt.Size())
	proof, err := smt.GetProof(0)
	if err != nil {
		t.Fatal(err)
	}
	if !smt.VerifyProof(0, proof) {
		t.Fatalf("verify proof of key [%d] failed", 0)
	}

	proof, err = smt.GetProof(200)
	if err != nil {
		t.Fatal(err)
	}
	if !smt.VerifyProof(200, proof) {
		t.Fatalf("verify proof of key [%d] failed", 200)
	}
}

func Test_BASSparseMerkleTree_GC(t *testing.T) {
	for _, env := range prepareEnv(t) {
		t.Logf("test [%s]", env.tag)
		testGC(t, env.hasher, env.db)
		env.db.Close()
	}
}
