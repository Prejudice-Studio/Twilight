import os

from sqlalchemy import create_engine, text

from src.config import Config


def create_database(database_name: str, model):
    os.makedirs(Config.DATABASES_DIR, exist_ok=True)
    database_url = f"sqlite:///{os.path.join(Config.DATABASES_DIR, f'{database_name}.db')}"
    engine = create_engine(database_url)
    model.metadata.create_all(engine)
    connection = engine.connect()
    connection.execute(text('PRAGMA journal_mode = WAL'))
    connection.close()