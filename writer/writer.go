package writer

type IWriter interface {
	Write(folder string, fileName string, data string) error
	Remove(folder string, fileName string) error
	IsJSON(fileName string) bool
}
