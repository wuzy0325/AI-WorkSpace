# Wind-DAQ Desktop

Tauri desktop shell for the Wind-DAQ Vue 3 interface.

The Tauri crate owns desktop capabilities only: window lifecycle, native dialogs,
asset hosting, and thin commands that call the Rust backend service. Business
logic belongs in `../../services/api-rs`.
