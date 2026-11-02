import Foundation

/// LineSplitter cuts a byte stream into newline-terminated lines. It is fed from one
/// readability handler at a time, which FileHandle runs serially, so it needs no lock.
final class LineSplitter: @unchecked Sendable {
    /// buffer holds bytes after the last newline.
    private var buffer = Data()
    /// onLine receives each non-empty line without its newline.
    private let onLine: @Sendable (Data) -> Void

    /// Creates a splitter that calls onLine per line.
    init(onLine: @escaping @Sendable (Data) -> Void) {
        self.onLine = onLine
    }

    /// feed appends chunk and emits every complete line.
    func feed(_ chunk: Data) {
        buffer.append(chunk)
        while let newline = buffer.firstIndex(of: 0x0A) {
            let line = buffer[buffer.startIndex..<newline]
            buffer.removeSubrange(buffer.startIndex...newline)
            if !line.isEmpty { onLine(Data(line)) }
        }
    }

    /// finish emits any trailing bytes that had no newline.
    func finish() {
        if !buffer.isEmpty { onLine(buffer) }
        buffer = Data()
    }

    /// attach reads handle until end of file, feeding the splitter, then calls onEnd.
    func attach(to handle: FileHandle, onEnd: @escaping @Sendable () -> Void) {
        handle.readabilityHandler = { [self] h in
            let chunk = h.availableData
            if chunk.isEmpty {
                h.readabilityHandler = nil
                finish()
                onEnd()
                return
            }
            feed(chunk)
        }
    }
}
