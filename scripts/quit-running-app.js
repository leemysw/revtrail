// Request a normal macOS quit so Revtrail can prompt about unsaved files.
ObjC.import("AppKit");
ObjC.bindFunction("kill", ["int", ["int", "int"]]);

function run([identifier]) {
    if (!identifier) throw new Error("Missing application bundle identifier.");
    const apps = $.NSRunningApplication.runningApplicationsWithBundleIdentifier(identifier);
    for (let i = 0; i < apps.count; i++) {
        const app = apps.objectAtIndex(i);
        const pid = app.processIdentifier;
        if ($.kill(pid, 0) !== 0) continue;
        if (!app.terminate) throw new Error("Revtrail declined the quit request.");
        const deadline = Date.now() + 60000;
        while ($.kill(pid, 0) === 0) {
            if (Date.now() >= deadline) {
                throw new Error("Revtrail is still open. Finish or cancel the quit dialog, then run make app-run again.");
            }
            delay(0.1);
        }
    }
}
