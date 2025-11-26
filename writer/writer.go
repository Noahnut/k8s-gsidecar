package writer

type IWriter interface {
	Write(fileName string, data string) error
	Remove(fileName string) error
	IsJSON(fileName string) bool
}
