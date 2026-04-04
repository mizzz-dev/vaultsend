package store

import "github.com/jackc/pgx/v5/pgxpool"

// Queries は sqlc 生成物を置く想定の型。
// 現時点は手書きの最小実装を同名で用意し、次PR以降で sqlc generate に置き換える。
type Queries struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Queries {
	return &Queries{db: db}
}
