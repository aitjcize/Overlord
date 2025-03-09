.PHONY: all build clean format lint install-tools test test-ios test-ui test-all test-verbose xcpretty

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

# Install xcpretty if needed
xcpretty:
	@if ! command -v xcpretty >/dev/null; then \
		echo "Installing xcpretty..."; \
		gem install --user-install xcpretty || sudo gem install xcpretty || echo "Failed to install xcpretty. Please install it manually with 'gem install xcpretty' or 'sudo gem install xcpretty'."; \
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

# Run tests (defaults to iOS simulator)
test: test-ios

# Run tests on iOS simulator
test-ios: xcpretty
	@echo "Running tests on iOS simulator..."
	@if command -v xcpretty >/dev/null; then \
		xcodebuild test -scheme overlord-app-ios -destination 'platform=iOS Simulator,name=iPhone 16 Pro Max' -only-testing:overlord-app-iosTests | xcpretty || \
		echo "Error: Could not run tests on iPhone 16 Pro Max simulator. Please ensure it is available."; \
	else \
		echo "xcpretty not available, running with standard output..."; \
		xcodebuild test -scheme overlord-app-ios -destination 'platform=iOS Simulator,name=iPhone 16 Pro Max' -only-testing:overlord-app-iosTests || \
		echo "Error: Could not run tests on iPhone 16 Pro Max simulator. Please ensure it is available."; \
	fi

# Run UI tests (requires iOS simulator)
test-ui: xcpretty
	@echo "Running UI tests on iOS simulator..."
	@if command -v xcpretty >/dev/null; then \
		xcodebuild test -scheme overlord-app-ios -destination 'platform=iOS Simulator,name=iPhone 16 Pro Max' -only-testing:overlord-app-iosUITests | xcpretty || \
		echo "Error: Could not run UI tests on iPhone 16 Pro Max simulator. Please ensure it is available."; \
	else \
		echo "xcpretty not available, running with standard output..."; \
		xcodebuild test -scheme overlord-app-ios -destination 'platform=iOS Simulator,name=iPhone 16 Pro Max' -only-testing:overlord-app-iosUITests || \
		echo "Error: Could not run UI tests on iPhone 16 Pro Max simulator. Please ensure it is available."; \
	fi

# Run all tests (both unit and UI tests)
test-all: test-ios test-ui
	@echo "All tests completed."

# Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	xcodebuild test -scheme overlord-app-ios -destination 'platform=iOS Simulator,name=iPhone 16 Pro Max' -only-testing:overlord-app-iosTests
