package main

import (
	"fmt"
	"gol/tplEngin/i18n"
)

func main() {
	lang := i18n.Load("lang", true)

	fmt.Println("ru")
	replacer := lang.NewReplacer("ru")
	fmt.Println(string(replacer.P("message")))
	fmt.Println(string(replacer.P("message1", "поле1", float64(2))))
	fmt.Println(string(replacer.Pf(1, "поле1", float64(2))))
	fmt.Println(2.5, replacer.Plural("apple", float64(2.5)))

	fmt.Println("en")
	replacer = lang.NewReplacer("en")
	fmt.Println(string(replacer.P("message")))
	fmt.Println(string(replacer.P("message1", "поле1", float64(2))))
	fmt.Println(2.5, replacer.Plural("apple", float64(2.5)))

}
