import Foundation

/// Backend runs `icelos serve` as a child process and exchanges JSON lines with it.
@MainActor
public final class Backend {
    /// executable is the icelos binary.
    public let executable: URL
    /// onMessage receives every decoded message.
    public var onMessage: ((Message) -> Void)?
    /// onExit receives a description when the process ends.
    public var onExit: ((String) -> Void)?

    /// process is the running child.
    private var process: Process?
    /// input is the child's stdin.
    private var input: FileHandle?
    /// splitters read the child's stdout and stderr.
    private var splitters: [LineSplitter] = []
    /// stderrTail keeps the end of the child's stderr for exit messages.
    private var stderrTail = ""
    /// openEnds counts the pipes and process still running, so exit is reported last.
    private var openEnds = 0
    /// exitCode is the child's status once it ends.
    private var exitCode: Int32 = 0

    /// Creates a backend for the binary at executable.
    public init(executable: URL) {
        self.executable = executable
    }

    /// locate finds the icelos binary: ICELOS_BACKEND, then the app bundle, then common
    /// install folders.
    public static func locate(environment: [String: String] = ProcessInfo.processInfo.environment,
                              bundle: Bundle = .main) -> URL? {
        let fm = FileManager.default
        var candidates: [URL] = []
        if let path = environment["ICELOS_BACKEND"], !path.isEmpty {
            candidates.append(URL(fileURLWithPath: path))
        }
        candidates.append(bundle.bundleURL.appending(path: "Contents/Helpers/icelos"))
        let home = fm.homeDirectoryForCurrentUser.path
        for dir in ["/opt/homebrew/bin", "/usr/local/bin", home + "/go/bin"] {
            candidates.append(URL(fileURLWithPath: dir).appending(path: "icelos"))
        }
        return candidates.first { fm.isExecutableFile(atPath: $0.path) }
    }

    /// start launches the child. Closing its stdin through stop makes it cancel and exit.
    public func start() throws {
        signal(SIGPIPE, SIG_IGN)
        let proc = Process()
        proc.executableURL = executable
        proc.arguments = ["serve"]
        let stdin = Pipe(), stdout = Pipe(), stderr = Pipe()
        proc.standardInput = stdin
        proc.standardOutput = stdout
        proc.standardError = stderr
        proc.terminationHandler = { [weak self] p in
            let code = p.terminationStatus
            DispatchQueue.main.async {
                MainActor.assumeIsolated {
                    self?.exitCode = code
                    self?.ended()
                }
            }
        }
        openEnds = 3
        try proc.run()
        process = proc
        input = stdin.fileHandleForWriting

        let decoder = JSONDecoder()
        let out = LineSplitter { [weak self] line in
            guard let msg = try? decoder.decode(Message.self, from: line) else { return }
            DispatchQueue.main.async { MainActor.assumeIsolated { self?.onMessage?(msg) } }
        }
        let end: @Sendable () -> Void = { [weak self] in
            DispatchQueue.main.async { MainActor.assumeIsolated { self?.ended() } }
        }
        out.attach(to: stdout.fileHandleForReading, onEnd: end)
        let err = LineSplitter { [weak self] line in
            let text = String(decoding: line, as: UTF8.self)
            DispatchQueue.main.async {
                MainActor.assumeIsolated {
                    guard let self else { return }
                    self.stderrTail = String((self.stderrTail + text + "\n").suffix(2048))
                }
            }
        }
        err.attach(to: stderr.fileHandleForReading, onEnd: end)
        splitters = [out, err]
    }

    /// send writes one request line. Writes after the child exits are dropped.
    public func send(_ request: Request) {
        guard let input, var data = try? JSONEncoder().encode(request) else { return }
        data.append(0x0A)
        try? input.write(contentsOf: data)
    }

    /// stop closes the child's stdin so it cancels running jobs and exits.
    public func stop() {
        try? input?.close()
        input = nil
    }

    /// ended counts down the open pipes and process and reports the exit after the last one,
    /// so every message and the full stderr tail arrive first.
    private func ended() {
        openEnds -= 1
        guard openEnds == 0 else { return }
        input = nil
        process = nil
        splitters = []
        let tail = stderrTail.trimmingCharacters(in: .whitespacesAndNewlines)
        onExit?(tail.isEmpty ? "The converter stopped (exit \(exitCode))." : tail)
    }
}
