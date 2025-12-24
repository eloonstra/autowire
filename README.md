# autowire

Compile-time dependency injection for Go using annotations.

## Installation

```bash
go install github.com/eloonstra/autowire@latest
```

## Usage

Run directly or via `go generate`:

```bash
autowire --out ./cmd --scan ./internal --scan ./pkg
```

```go
//go:generate go run github.com/eloonstra/autowire --out ./cmd --scan ./internal --scan ./pkg
```

## Annotations

```go
//autowire:provide
func NewConfig() *Config { ... }

//autowire:provide
func NewDatabase(cfg *Config) (*Database, error) { ... }

//autowire:provide
type Server struct {
    Config *Config   // injected (exported fields only)
}

//autowire:invoke
func SetupRoutes(svc *UserService) error { ... }
```

- `//autowire:provide`: registers a single type as injectable (functions or structs)
- `//autowire:invoke`: calls a function during initialization for side effects

Functions can optionally return an error.

### Interface Binding

Bind a provider to an interface instead of its concrete type:

```go
//autowire:provide Reader
func NewFileReader() *FileReader { ... }
```

For interfaces from other packages, import the package and use the package alias:

```go
import "io"

var _ io.Writer = (*BufferedWriter)(nil) // compile-time check

//autowire:provide io.Writer
func NewBufferedWriter() *BufferedWriter { ... }
```

Adding `var _ interfaces.Interface = (*Implementation)(nil)` is recommended. it verifies at compile time that your type implements the interface and prevents "imported and not used" errors.

- `InterfaceName`: interface in same package
- `package.InterfaceName`: imported interface (requires import)

## Generated Output

Generates `app_gen.go` with an `App` struct containing all providers and an `InitializeApp()` function that wires
everything in dependency order:

```go
func main() {
    app, err := InitializeApp()
    if err != nil {
        panic(err)
    }
    app.Server.Start()
}
```

## Comparison

|                    |   autowire    | wire (archived) | do | Fx |
|--------------------|:-------------:|:---------------:|:--:|:--:|
| Compile-time safe  |       ✓       |        ✓        | ✓  |    |
| Code generation    |       ✓       |        ✓        |    |    |
| No manual wiring   |       ✓       |                 |    |    |
| Struct injection   |       ✓       |                 |    |    |
| Zero runtime cost  |       ✓       |        ✓        |    |    |
| Cycle detection    |       ✓       |        ✓        | ✓  | ✓  |
| Interface binding  |       ✓       |        ✓        | ✓  | ✓  |
| Lifecycle hooks    |               |                 | ✓  | ✓  |
| Named dependencies |     soon™     |        ✓        | ✓  | ✓  |

- **[wire](https://github.com/google/wire)**: Google's compile-time DI, requires manual provider sets (archived)
- **[do](https://github.com/samber/do)**: Lightweight runtime DI container with generics
- **[Fx](https://github.com/uber-go/fx)**: Uber's application framework with lifecycle management

## License

[MIT](LICENSE)
