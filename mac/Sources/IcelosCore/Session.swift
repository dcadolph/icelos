import Foundation
import Observation
import os

/// Session connects the window model to the converter process. Files opened before the
/// converter starts, such as those Finder hands over at launch, wait and are added once it runs.
@MainActor
@Observable
public final class Session {
    /// model is the window state.
    public let model: Converter
    /// backend is the converter process, or nil when the binary is missing.
    @ObservationIgnored private let backend: Backend?
    /// waiting holds files opened before start, in the order they arrived.
    @ObservationIgnored private var waiting: [URL] = []
    /// started is true once start has run.
    @ObservationIgnored private var started = false
    /// log records the session's milestones in the unified log.
    @ObservationIgnored private let log = Logger(subsystem: "com.dcadolph.icelos", category: "session")

    /// Creates a session with the converter Backend.locate finds and the user's settings.
    public convenience init() {
        self.init(backend: Backend.locate().map { Backend(executable: $0) }, settings: AppSettings())
    }

    /// Creates a session around backend, which is nil when no converter was found.
    public init(backend: Backend?, settings: AppSettings) {
        self.backend = backend
        let model = Converter(send: { backend?.send($0) }, preferences: settings)
        model.outputFolder = settings.outputFolder
        model.hardware = settings.useMediaEngine
        self.model = model
    }

    /// open adds urls to the window, or holds them until start when the converter is not
    /// running yet.
    public func open(urls: [URL]) {
        guard started else {
            waiting.append(contentsOf: urls)
            return
        }
        model.add(urls: urls)
    }

    /// start launches the converter once, then adds the files opened before it.
    public func start() {
        guard !started else { return }
        started = true
        defer {
            let urls = waiting
            waiting = []
            model.add(urls: urls)
        }
        guard let backend else {
            model.backendExited("The icelos converter is missing. Reinstall the app, or set ICELOS_BACKEND.")
            return
        }
        let model = model
        let log = log
        let path = backend.executable.path
        backend.onMessage = { message in
            if message.type == "ready" {
                log.notice("backend ready: \(path, privacy: .public) version \(message.version ?? "", privacy: .public)")
            }
            model.handle(message)
        }
        backend.onExit = { model.backendExited($0) }
        do {
            try backend.start()
        } catch {
            model.backendExited("The converter could not start: \(error.localizedDescription)")
        }
    }

    /// stop ends the converter, canceling running jobs.
    public func stop() {
        backend?.stop()
    }
}
