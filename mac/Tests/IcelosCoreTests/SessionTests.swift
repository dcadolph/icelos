import Foundation
import Testing

@testable import IcelosCore

/// FakeServer is a converter script that says ready and writes every request it reads to a log.
@MainActor
struct FakeServer {
    /// dir holds the script and its log.
    let dir: URL
    /// backend runs the script.
    let backend: Backend
    /// log is where the script writes each request line.
    let log: URL

    /// Writes the script into a fresh temporary folder.
    init() throws {
        dir = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        log = dir.appending(path: "requests.log")
        let bin = dir.appending(path: "icelos")
        let script = """
        #!/bin/sh
        echo '{"type":"ready","version":"fake"}'
        while read -r line; do printf '%s\\n' "$line" >> '\(log.path)'; done
        """
        try Data(script.utf8).write(to: bin)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: bin.path)
        backend = Backend(executable: bin)
    }

    /// probedPaths returns the paths of the probe requests the script has read so far.
    func probedPaths() -> [String] {
        guard let text = try? String(contentsOf: log, encoding: .utf8) else { return [] }
        return text.split(separator: "\n").compactMap { line in
            guard let req = try? JSONDecoder().decode(Request.self, from: Data(line.utf8)), req.op == "probe" else {
                return nil
            }
            return req.path
        }
    }

    /// waitForProbes waits up to two seconds for count probe requests and returns their paths.
    func waitForProbes(_ count: Int) async throws -> [String] {
        for _ in 0..<200 where probedPaths().count < count { try await Task.sleep(for: .milliseconds(10)) }
        return probedPaths()
    }

    /// Removes the folder.
    func remove() {
        try? FileManager.default.removeItem(at: dir)
    }
}

@MainActor
@Suite struct SessionTests {
    /// settings returns settings in a throwaway defaults domain.
    func settings() -> AppSettings {
        let suite = "icelos.tests." + UUID().uuidString
        let defaults = UserDefaults(suiteName: suite)!
        defaults.removePersistentDomain(forName: suite)
        return AppSettings(defaults: defaults)
    }

    @Test func filesOpenedBeforeStartWaitForIt() async throws {
        let server = try FakeServer()
        defer { server.remove() }
        let session = Session(backend: server.backend, settings: settings())
        let a = URL(fileURLWithPath: "/tmp/a.mov")
        let b = URL(fileURLWithPath: "/tmp/b.mov")
        session.open(urls: [a, b])
        #expect(session.model.items.isEmpty, "nothing is added before the converter starts")
        session.start()
        #expect(session.model.items.map(\.url) == [a, b])
        #expect(try await server.waitForProbes(2) == [a.path, b.path])
        session.start()
        #expect(session.model.items.count == 2, "a second start adds nothing again")
        session.stop()
    }

    @Test func filesOpenedAfterStartAreAddedAtOnce() async throws {
        let server = try FakeServer()
        defer { server.remove() }
        let session = Session(backend: server.backend, settings: settings())
        session.start()
        let a = URL(fileURLWithPath: "/tmp/a.mov")
        session.open(urls: [a])
        #expect(session.model.items.map(\.url) == [a])
        #expect(try await server.waitForProbes(1) == [a.path])
        session.stop()
    }

    @Test func duplicatesAreAddedOnce() async throws {
        let server = try FakeServer()
        defer { server.remove() }
        let session = Session(backend: server.backend, settings: settings())
        let a = URL(fileURLWithPath: "/tmp/a.mov")
        session.open(urls: [a])
        session.open(urls: [a])
        session.start()
        session.open(urls: [a])
        #expect(session.model.items.map(\.url) == [a])
        _ = try await server.waitForProbes(1)
        try await Task.sleep(for: .milliseconds(200))
        #expect(server.probedPaths() == [a.path], "the converter is asked about the file once")
        session.stop()
    }

    @Test func missingConverterStillListsTheFiles() {
        let session = Session(backend: nil, settings: settings())
        let a = URL(fileURLWithPath: "/tmp/a.mov")
        session.open(urls: [a])
        session.start()
        #expect(session.model.items.map(\.url) == [a])
        #expect(session.model.fault != nil)
    }
}
