from locust import HttpUser, task, between
import random
import time

class WarUser(HttpUser):
    wait_time = between(1, 2)

    @task
    def send_report(self):
        countries = ["USA", "RUS", "CHN", "ESP", "GTM"]

        payload = {
            "country": random.choice(countries),
            "warplanes_in_air": random.randint(0, 50),
            "warships_in_water": random.randint(0, 30),
            "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ")
        }

        self.client.post("/grpc-202102140", json=payload)