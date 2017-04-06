package main

import (
	"flag"
	"fmt"
	"github.com/melonwool/iris-binding"
	"gopkg.in/kataras/iris.v6"
	"gopkg.in/kataras/iris.v6/adaptors/httprouter"
)

type User struct {
	Name    string `form:"name" binding:"required"`
	Age     int    `form:"age" binding:"required"`
	Email   string `form:"email" binding:"required"`
	Address string `form:"address" binding:"required"`
}

func main() {
	port := flag.String("port", "8080", "http listen port")
	flag.Parse()
	app := iris.New()
	app.Adapt(
		iris.DevLogger(),
		httprouter.New(),
	)
	app.Post("/user/add", UserAdd)

	app.Listen(":" + *port)
}

func UserAdd(ctx *iris.Context) {
	userInterface, _ := binding.Form(ctx, User{})
	user := userInterface.(User)
	fmt.Printf("The Form value is %+v\n", user)
}
