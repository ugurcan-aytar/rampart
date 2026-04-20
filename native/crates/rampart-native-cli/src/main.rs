//! rampart-native binary — thin entrypoint that binds a Unix Domain
//! Socket and dispatches to the library crate's server loop.
//!
//! Config is environment-driven so the Docker Compose story (Adım 7)
//! can wire a shared volume without rebuilding the image:
//!
//!     RAMPART_NATIVE_SOCKET   — UDS path (default /tmp/rampart-native.sock)
//!     RAMPART_LOG_FORMAT      — `text` (default) or `json`
//!     RUST_LOG                — tracing env-filter spec (default `info`)

use std::env;

use anyhow::Context;
use tokio::signal::unix::{signal, SignalKind};
use tracing_subscriber::EnvFilter;

fn main() -> anyhow::Result<()> {
    init_tracing();

    let rt = tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .context("build single-threaded tokio runtime")?;

    rt.block_on(async_main())
}

async fn async_main() -> anyhow::Result<()> {
    let socket_path = env::var("RAMPART_NATIVE_SOCKET")
        .unwrap_or_else(|_| "/tmp/rampart-native.sock".to_string());
    tracing::info!(socket_path, "starting rampart-native");

    let server = rampart_native::ipc::serve(&socket_path)
        .await
        .context("bind UDS listener")?;

    // SIGTERM → graceful shutdown.
    let mut sigterm = signal(SignalKind::terminate()).context("install SIGTERM handler")?;
    let mut sigint = signal(SignalKind::interrupt()).context("install SIGINT handler")?;

    tokio::select! {
        _ = sigterm.recv() => tracing::info!("SIGTERM received"),
        _ = sigint.recv()  => tracing::info!("SIGINT received"),
    }

    server.shutdown().await;
    tracing::info!("rampart-native exited cleanly");
    Ok(())
}

fn init_tracing() {
    let filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info"));
    let json = env::var("RAMPART_LOG_FORMAT").ok().as_deref() == Some("json");
    let builder = tracing_subscriber::fmt()
        .with_env_filter(filter)
        .with_target(false);
    if json {
        builder.json().init();
    } else {
        builder.init();
    }
}
