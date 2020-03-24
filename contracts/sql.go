package contracts

type RowScanner interface {
	Scan(dest ...interface{}) error
}
