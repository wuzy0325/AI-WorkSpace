//! Application orchestration.
//!
//! Use cases coordinate `core` logic through `ports` traits. They must not call
//! concrete hardware, persistence, or messaging adapters directly.
