from locust import HttpUser, task, between
from datetime import datetime, timezone
import random

PASES_VALIDOS = ["USA", "RUS", "CHN", "ESP", "GTM"]
CARNET = "202102140"

class UsuarioReporteMilitar(HttpUser):
    wait_time = between(0.2, 0.8)

    @task
    def enviar_reporte(self):
        reporte = {
            "country": random.choice(PASES_VALIDOS),
            "warplanes_in_air": random.randint(0, 50),
            "warships_in_water": random.randint(0, 30),
            "timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        }

        with self.client.post(
            f"/grpc-{CARNET}",
            json=reporte,
            catch_response=True,
        ) as respuesta:
            if respuesta.status_code == 200:
                respuesta.success()
            else:
                respuesta.failure(
                    f"Error {respuesta.status_code}: {respuesta.text[:150]}"
                )