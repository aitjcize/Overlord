#if os(iOS)
    import UIKit
#elseif os(macOS)
    import AppKit
#endif
import CoreGraphics
import SwiftTerm
import SwiftUI

// Define a struct to replace the large tuple
private struct HexColorComponents {
    let red: UInt64
    let green: UInt64
    let blue: UInt64
    let alpha: UInt64
}

// Shared hex parsing function to reduce duplication
private func parseHexColor(_ hex: String) -> HexColorComponents {
    let hex = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
    var int: UInt64 = 0
    Scanner(string: hex).scanHexInt64(&int)

    switch hex.count {
    case 3: // RGB (12-bit)
        return HexColorComponents(
            red: (int >> 8) * 17,
            green: (int >> 4 & 0xF) * 17,
            blue: (int & 0xF) * 17,
            alpha: 255
        )
    case 6: // RGB (24-bit)
        return HexColorComponents(red: int >> 16, green: int >> 8 & 0xFF, blue: int & 0xFF, alpha: 255)
    case 8: // ARGB (32-bit)
        return HexColorComponents(red: int >> 16 & 0xFF, green: int >> 8 & 0xFF, blue: int & 0xFF, alpha: int >> 24)
    default:
        return HexColorComponents(red: 0, green: 0, blue: 0, alpha: 255)
    }
}

extension SwiftUI.Color {
    init(hex: String) {
        let components = parseHexColor(hex)
        self.init(
            .sRGB,
            red: Double(components.red) / 255,
            green: Double(components.green) / 255,
            blue: Double(components.blue) / 255,
            opacity: Double(components.alpha) / 255
        )
    }
}

#if os(iOS)
    extension UIColor {
        convenience init(hex: String) {
            let components = parseHexColor(hex)
            self.init(
                red: CGFloat(components.red) / 255,
                green: CGFloat(components.green) / 255,
                blue: CGFloat(components.blue) / 255,
                alpha: CGFloat(components.alpha) / 255
            )
        }
    }
#endif

extension SwiftTerm.Color {
    convenience init(hex: String) {
        let components = parseHexColor(hex)
        // Scale from 0-255 to 0-65535 for SwiftTerm.Color
        self.init(
            red: UInt16(components.red) * 257,
            green: UInt16(components.green) * 257,
            blue: UInt16(components.blue) * 257
        )
    }
}
