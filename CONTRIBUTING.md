# Contributing

Contributions are welcome! Here's how to get started:

1. **Fork** the repository on GitHub (click the "Fork" button on the repo page) and then clone your fork:
   ```bash
   git clone https://github.com/<your-username>/azdo.git
   cd azdo
   ```

2. **Create a branch** for your changes:
   ```bash
   git checkout -b feature/my-change
   ```

3. **Develop** using the standard Go workflow:
   ```bash
   go build -o azdo ./cmd/azdo-tui   # Build
   go test ./...                      # Run tests
   go fmt ./...                       # Format code
   go vet ./...                       # Check for issues
   ```

4. **Push** your branch to your fork and **open a pull request** against the `main` branch on GitHub.

For architecture details and code organization, see [ARCHITECTURE.md](ARCHITECTURE.md).
