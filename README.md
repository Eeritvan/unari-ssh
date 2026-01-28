## ssh unari
A cli tool to browser unicafe menus via ssh

### production
open terminal and try typing `ssh unari.eeritvan.dev`

### developing locally
1. [install Go](https://go.dev/doc/install) (1.25.4+)
2. Clone this repo
3. download dependencies with `go mod download`
4. create .env (see .env.template for required values)
5. launch server with `go run .`
6. connect to the app with another terminal using `ssh localhost -p <PORT>`
