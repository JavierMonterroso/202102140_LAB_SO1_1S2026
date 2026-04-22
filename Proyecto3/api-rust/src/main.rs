use axum::{
    extract::State,
    http::StatusCode,
    routing::{get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tokio::net::TcpListener;
use tracing::{info, warn};

const CARNET: &str = "202102140";

#[derive(Debug, Deserialize, Serialize, Clone)]
struct ReporteGuerra {
    country: String,
    warplanes_in_air: i32,
    warships_in_water: i32,
    timestamp: String,
}

#[derive(Debug, Serialize)]
struct RespuestaApi {
    estado: String,
    mensaje: String,
}

#[derive(Clone)]
struct EstadoApp {
    url_cliente_go: String,
    cliente_http: reqwest::Client,
}

async fn salud() -> &'static str {
    "OK"
}

fn pais_valido(pais: &str) -> bool {
    matches!(pais, "USA" | "RUS" | "CHN" | "ESP" | "GTM")
}

fn validar_reporte(reporte: &ReporteGuerra) -> Result<(), String> {
    if !pais_valido(&reporte.country) {
        return Err("País no válido. Use: USA, RUS, CHN, ESP o GTM".to_string());
    }

    if !(0..=50).contains(&reporte.warplanes_in_air) {
        return Err("warplanes_in_air debe estar entre 0 y 50".to_string());
    }

    if !(0..=30).contains(&reporte.warships_in_water) {
        return Err("warships_in_water debe estar entre 0 y 30".to_string());
    }

    if reporte.timestamp.trim().is_empty() {
        return Err("timestamp no puede ir vacío".to_string());
    }

    Ok(())
}

async fn recibir_reporte(
    State(estado): State<Arc<EstadoApp>>,
    Json(reporte): Json<ReporteGuerra>,
) -> (StatusCode, Json<RespuestaApi>) {
    info!(
        "Reporte recibido: país={} aviones={} barcos={}",
        reporte.country, reporte.warplanes_in_air, reporte.warships_in_water
    );

    if let Err(error) = validar_reporte(&reporte) {
        return (
            StatusCode::BAD_REQUEST,
            Json(RespuestaApi {
                estado: "error".to_string(),
                mensaje: error,
            }),
        );
    }

    let mut acumulador: u64 = 0;
    for i in 0..2_000_000u64 {
        acumulador = acumulador.wrapping_add(i);
    }
    let _ = acumulador;

    let url = format!("{}/procesar", estado.url_cliente_go);

    match estado.cliente_http.post(&url).json(&reporte).send().await {
        Ok(respuesta) => {
            if respuesta.status().is_success() {
                (
                    StatusCode::OK,
                    Json(RespuestaApi {
                        estado: "ok".to_string(),
                        mensaje: "Reporte recibido y enviado a Go correctamente".to_string(),
                    }),
                )
            } else {
                (
                    StatusCode::BAD_GATEWAY,
                    Json(RespuestaApi {
                        estado: "error".to_string(),
                        mensaje: format!(
                            "El cliente Go respondió con estado {}",
                            respuesta.status()
                        ),
                    }),
                )
            }
        }
        Err(error) => {
            warn!("No se pudo contactar al cliente Go: {}", error);
            (
                StatusCode::BAD_GATEWAY,
                Json(RespuestaApi {
                    estado: "error".to_string(),
                    mensaje: "No se pudo reenviar el reporte al cliente Go".to_string(),
                }),
            )
        }
    }
}

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt::init();

    let url_cliente_go = std::env::var("URL_CLIENTE_GO")
        .unwrap_or_else(|_| "http://127.0.0.1:8081".to_string());

    info!("URL cliente Go: {}", url_cliente_go);

    let estado = Arc::new(EstadoApp {
        url_cliente_go,
        cliente_http: reqwest::Client::new(),
    });

    let ruta_reporte = format!("/grpc-{}", CARNET);

    let app = Router::new()
        .route("/", get(salud))
        .route("/health", get(salud))
        .route(&ruta_reporte, post(recibir_reporte))
        .with_state(estado);

    let listener = TcpListener::bind("0.0.0.0:8080")
        .await
        .expect("No se pudo abrir el puerto 8080");

    info!(
        "API Rust escuchando en 8080 | endpoint principal: POST {}",
        ruta_reporte
    );

    axum::serve(listener, app)
        .await
        .expect("No se pudo iniciar el servidor Rust");
}