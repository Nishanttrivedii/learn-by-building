---
id: m00-l1
title: Running Go
milestone: m00
---

Before writing any shipment code you need three habits: run a program, let Go check it, and print values to see what's going on. This lesson covers the tools you'll use on every milestone, how Go splits code into packages, and how `fmt` prints things.

Every example here was run for real. Its output is exactly what Go printed.

## concept go.toolchain: Go checks your whole program before it runs any of it
flags: core, new

### Example
```go
package main

import "fmt"

func main() {
	fmt.Println("Searching London to Manchester...")
	fmt.Printn("Found 12 shipments")
}
```
```output
./main.go:7:6: undefined: fmt.Printn
```

### Explain
`go run .` first compiles every file, then runs the result. Line 7 misspells `Println`, so compiling fails and **nothing runs**, not even the correct line 6 above it. The message names the file, line and column (`main.go:7:6`). You fix mistakes before the program starts, not when a user reaches that line.

### In JavaScript
```js
console.log("Searching London to Manchester...");
try {
  console.lg("Found 12 shipments");
} catch (e) {
  console.log("Crashed:", e.message);
}
```
```output
Searching London to Manchester...
Crashed: console.lg is not a function
```
JavaScript runs line 1 and only discovers the typo when it gets to line 2. In Go, a typo in a rarely used branch still stops the build, so it can't hide until production.

### More
Some mistakes compile fine but are still wrong. `go vet` catches the common ones, for example a `%d` (number) placeholder given a string:
```go run=vet
package main

import "fmt"

func main() {
	fmt.Printf("Rate: %d\n", "4257")
}
```
```output
main.go:6:20: fmt.Printf format %d has arg "4257" of wrong type string
```
The tools you'll use all the time:
- `go run .`: compile and run.
- `go build`: compile into a program file.
- `go vet`: look for suspicious code.
- `go test`: run tests. It also runs part of `go vet` first.
- `gofmt`: rewrite code into the one standard layout, so nobody argues about formatting.

On this Windows checkout, `gofmt -l .` at the root of shipping-service lists about 300 files. That's only Windows line endings, not real formatting problems.

### Takeaway
Go compiles everything first, so a typo anywhere stops the whole program; `go vet` catches mistakes that compile but are wrong.

## concept go.packages: A capitalised name can be used from other packages; a lowercase one cannot
flags: core, differs

### Example
```go file=rate/rate.go
package rate

// Total is exported: other packages can call rate.Total.
func Total(base, tax int) int {
	return base + roundUp(tax)
}

// roundUp is unexported: only code inside package rate can call it.
func roundUp(n int) int {
	return (n + 9) / 10 * 10
}
```
```go
package main

import (
	"fmt"

	"example/rate"
)

func main() {
	fmt.Println(rate.Total(4257, 503))
}
```
```output
4767
```

### Explain
Each folder is one package. `rate/rate.go` declares `package rate`, and `main.go` imports it by its path, `example/rate` (`example` is the module name from `go.mod`). `Total` starts with a capital letter, so `main` can call it. `roundUp` starts lowercase, so it's private to `package rate`, but `Total` can still use it. 503 rounds up to 510, and 4257 + 510 = 4767.

### In JavaScript
In JavaScript you choose what leaves a module with the `export` keyword:
```js snippet
// rate.js
const roundUp = (n) => Math.ceil(n / 10) * 10; // private: not exported
export const total = (base, tax) => base + roundUp(tax);
```
Go has no `export` keyword: the first letter of the name decides.

### More
The first letter decides for everything, not just functions: types, constants, even the fields of a struct. That's why the hotel code writes `ProviderCode` but `masterToProvider`.

Calling the lowercase function from outside its package doesn't compile:
```go file=rate/rate.go
package rate

func roundUp(n int) int {
	return (n + 9) / 10 * 10
}
```
```go
package main

import (
	"fmt"

	"example/rate"
)

func main() {
	fmt.Println(rate.roundUp(503))
}
```
```output
./main.go:10:19: undefined: rate.roundUp
```

### Takeaway
One folder = one package; capital first letter = usable from outside, lowercase = private to the package.

## concept go.fmt: fmt's verbs decide how a value is printed
flags: core, differs

### Example
```go
package main

import "fmt"

func main() {
	code := "LON"
	rate := 4257
	rate := 0.05

	fmt.Println(code, rate, rate)
	fmt.Printf("%v %v %v\n", code, rate, rate)
	fmt.Printf("%q %d %.2f\n", code, rate, rate)
	fmt.Printf("%T %T %T\n", code, rate, rate)
}
```
```output
LON 4257 0.05
LON 4257 0.05
"LON" 4257 0.05
string int float64
```

### Explain
`Println` prints its values separated by spaces. `Printf` fills a template instead. `%v` is "the value in its default form", `%q` puts a string in quotes (handy for spotting stray spaces), `%d` prints a whole number, `%.2f` a decimal to two places, and `%T` prints the **type**. The last line shows that `4257` is an `int` and `0.05` a `float64`: two different kinds of number.

### In JavaScript
```js
const code = "LON";
const rate = 4257;
const rate = 0.05;
console.log(code, rate, rate);
console.log(typeof code, typeof rate, typeof rate);
```
```output
LON 4257 0.05
string number number
```
JavaScript has one `number` type for both. Go keeps whole numbers (`int`) and decimals (`float64`) apart, and the next lesson shows why that matters.

### More
`fmt.Sprintf` fills the same template but **returns** the string instead of printing it. You'll use it to build text such as a rate line:
```go
package main

import "fmt"

func main() {
	line := fmt.Sprintf("%s-%s: %d INR", "LON", "MAN", 4257)
	fmt.Println(line)
	fmt.Println(len(line))
}
```
```output
LON-MAN: 4257 INR
17
```

### Takeaway
`%v` for any value, `%q` to see a string exactly, `%T` to see a type; `Sprintf` returns the text instead of printing it.

## exercise m00-l1-p1 predict: What do these verbs print?
practises: go.fmt

### Prompt
Type the exact line this program prints.
```go
package main

import "fmt"

func main() {
	fmt.Printf("%v|%q|%T|%d\n", "MAN", "MAN", 7, 7)
}
```

### Answer
```output
MAN|"MAN"|int|7
```

### Why
`%v` prints the string as it is, `%q` adds quotes, `%T` gives the type of `7` (`int`), and `%d` prints the number.

## exercise m00-l1-p2 choice: Does the first line print?
practises: go.toolchain

### Prompt
What happens when you run this with `go run .`?
```go
package main

import "fmt"

func main() {
	fmt.Println("Booking CYGFLT-1001")
	fmt.Println("Status:", status)
}
```

### Options
- [ ] It prints `Booking CYGFLT-1001`, then stops with an error on the next line
- [x] It prints nothing except an error, because `status` doesn't exist
- [ ] It prints `Booking CYGFLT-1001` and then `Status:` with an empty value

### Answer
```output
./main.go:7:25: undefined: status
```

### Why
Go compiles the whole program before running any of it. `status` was never declared, so compiling fails and even the first `Println` never runs. JavaScript would have printed the first line and then thrown a `ReferenceError`.

## exercise m00-l1-e1 fix: Make go vet happy
practises: go.toolchain, go.fmt
dir: m00/rateline

### Task
`Line` should format one rate for the results page, for example `Line("LON-MAN", 4257)` → `LON-MAN: 4257 INR`. It has a bug that `go vet` can spot.

1. Run `go vet ./practice/m00/rateline/` and read what it says.
2. Fix `rateline.go`.
3. Run `go test ./practice/m00/rateline/` until it passes.

Run both commands from the learning folder, `~/go-learning`.

### Hints
1. Read the vet message: it names the verb and the value that don't match.
2. `amount` is an `int`. Which verb prints a whole number?
3. Change `%s` to `%d` for `amount`, and keep `%s` for `route`.

### Why
`%s` is for strings. Given an int, `Sprintf` doesn't fail; it quietly prints `%!s(int=4257)`, which would end up on a user's screen. `go vet` (which `go test` runs automatically) catches the mismatch before that can happen.

## exercise m00-l1-e2 fix: Let another package use the function
practises: go.packages
dir: m00/depot

### Task
The test in `depot_test.go` sits in a **different** package (`depot_test`), the way the rest of the app would use this code. It calls `depot.DisplayName("Mumbai", "MAN")` and expects `Mumbai (MAN)`, but it doesn't compile.

Fix `depot.go` so the test compiles and passes, then run `go test ./practice/m00/depot/` from `~/go-learning`.

### Hints
1. Read the compile error: it tells you the name can't be used from outside.
2. Which letter decides whether a name leaves its package?
3. Rename `displayName` to `DisplayName`.

### Why
Tests written in a separate `_test` package can only see what a real caller would see. A lowercase `displayName` is invisible outside `package depot`, so renaming it with a capital letter is what makes it part of the package's public surface.
