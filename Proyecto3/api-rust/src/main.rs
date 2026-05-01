use axum::{
    http::StatusCode,
    response::IntoResponse,
    routing::{get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};
use std::env;
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    net::{TcpListener, TcpStream},
};

#[derive(Deserialize, Serialize, Debug)]
struct WarReport {
    country: String,
    warplanes_in_air: i32,
    warships_in_water: i32,
    timestamp: String,
}

async fn health_check() -> &'static str {
    "OK"
}

async fn forward_to_go(payload: &WarReport) -> Result<String, String> {
    let host = env::var("GO_INGEST_HOST").unwrap_or_else(|_| "go-ingest-service".to_string());
    let port = env::var("GO_INGEST_PORT").unwrap_or_else(|_| "80".to_string());
    let path = env::var("GO_INGEST_PATH").unwrap_or_else(|_| "/report".to_string());
    let addr = format!("{host}:{port}");
    let body = serde_json::to_string(payload).map_err(|err| err.to_string())?;

    println!("[RUST][2] Reenviando reporte a Go REST: http://{addr}{path}");

    let mut stream = TcpStream::connect(&addr)
        .await
        .map_err(|err| format!("no se pudo conectar a {addr}: {err}"))?;

    let request = format!(
        "POST {path} HTTP/1.1\r\nHost: {host}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.as_bytes().len()
    );

    stream
        .write_all(request.as_bytes())
        .await
        .map_err(|err| format!("no se pudo enviar reporte a Go: {err}"))?;

    let mut response = String::new();
    stream
        .read_to_string(&mut response)
        .await
        .map_err(|err| format!("no se pudo leer respuesta de Go: {err}"))?;

    let status_line = response.lines().next().unwrap_or("HTTP/1.1 000");
    let status_code = status_line.split_whitespace().nth(1).unwrap_or("000");

    if status_code.starts_with('2') {
        println!("[RUST][3] Go REST respondio correctamente: {status_line}");
        Ok(status_line.to_string())
    } else {
        println!("[RUST][ERROR] Go REST respondio con error: {status_line}");
        Err(format!("Go respondio con {status_line}"))
    }
}

async fn receive_report(Json(payload): Json<WarReport>) -> impl IntoResponse {
    let mut _dummy: u64 = 0;
    for i in 0..5_000_000 {
        _dummy = _dummy.wrapping_add(i);
    }

    println!(
        "[RUST][1] Recibido desde Locust/Gateway: pais={}, aviones={}, barcos={}, timestamp={}",
        payload.country, payload.warplanes_in_air, payload.warships_in_water, payload.timestamp
    );

    match forward_to_go(&payload).await {
        Ok(status) => (
            StatusCode::OK,
            format!("Reporte de {} recibido y enviado a Go ({status})", payload.country),
        )
            .into_response(),
        Err(err) => (
            StatusCode::BAD_GATEWAY,
            format!("Reporte recibido, pero fallo el envio a Go: {err}"),
        )
            .into_response(),
    }
}

#[tokio::main]
async fn main() {
    let app = Router::new()
        .route("/", get(health_check))
        .route("/grpc-202102140", post(receive_report));

    let listener = TcpListener::bind("0.0.0.0:8080").await.unwrap();

    println!("[RUST][BOOT] API Rust corriendo en puerto 8080");
    println!("[RUST][BOOT] Endpoint publico: POST /grpc-202102140");
    println!("[RUST][BOOT] Siguiente salto por DNS interno: go-ingest-service:80/report");

    axum::serve(listener, app).await.unwrap();
}
