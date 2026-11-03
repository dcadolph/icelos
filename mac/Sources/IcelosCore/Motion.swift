import SwiftUI

/// Motion decides whether an animation runs under the Reduce Motion setting.
public enum Motion {
    /// animation returns base, or nil when the user asked for reduced motion.
    public static func animation(_ base: Animation, reduceMotion: Bool) -> Animation? {
        reduceMotion ? nil : base
    }
}
