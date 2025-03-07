.PHONY: all build clean format lint install-tools

# Default target
all: build

# Install required tools
install-tools:
	@echo "Installing required tools..."
	@if ! command -v swiftformat >/dev/null; then \
		echo "Installing SwiftFormat..."; \
		brew install swiftformat; \
	fi
	@if ! command -v swiftlint >/dev/null; then \
		echo "Installing SwiftLint..."; \
		brew install swiftlint; \
	fi

# Build the project
build:
	@echo "Building project..."
	xcodebuild -scheme overlord-app-ios -sdk iphonesimulator -configuration Debug build

# Clean the project
clean:
	@echo "Cleaning project..."
	xcodebuild -scheme overlord-app-ios clean
	rm -rf build/
	rm -rf DerivedData/

# Format Swift code
format: install-tools
	@echo "Formatting Swift code..."
	swiftformat --config .swiftformat .

# Lint Swift code
lint: install-tools
	@echo "Linting Swift code..."
	swiftlint lint --quiet --config .swiftlint.yml
