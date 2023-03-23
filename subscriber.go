package rabbitmqsdk

type Subscriber interface {
	Listen()
	Close()
}
