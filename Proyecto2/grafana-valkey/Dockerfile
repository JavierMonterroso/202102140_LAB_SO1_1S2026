FROM grafana/grafana:11.0.0
# Instalamos el plugin necesario para leer desde Valkey/Redis
RUN grafana-cli plugins install redis-datasource
USER root
RUN mkdir -p /var/lib/grafana && chown -R grafana:grafana /var/lib/grafana
USER grafana