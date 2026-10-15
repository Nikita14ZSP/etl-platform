"""Общие фикстуры для тестов.

Поднимает реальный Postgres через testcontainers, применяет миграции
и отдаёт SQLAlchemy-сессию. Контейнер живёт на всю сессию pytest,
таблицы пересоздаются перед каждым тестом.
"""

import os
from collections.abc import Iterator

import pytest
from sqlalchemy import create_engine, text
from sqlalchemy.orm import Session, sessionmaker
from testcontainers.postgres import PostgresContainer


@pytest.fixture(scope="session")
def postgres_container() -> Iterator[PostgresContainer]:
    """Postgres в Docker на время всей сессии тестов."""
    with PostgresContainer("postgres:16") as container:
        yield container


@pytest.fixture(scope="session")
def database_url(postgres_container: PostgresContainer) -> str:
    """URL для SQLAlchemy. Драйвер psycopg (v3), как в проде."""
    raw = postgres_container.get_connection_url()
    # testcontainers отдаёт postgresql+psycopg2://..., нам нужен psycopg (v3)
    return raw.replace("postgresql+psycopg2://", "postgresql+psycopg://")


@pytest.fixture(scope="session")
def alembic_config(database_url: str) -> Iterator[None]:
    """Прогоняет alembic upgrade head на контейнере один раз за сессию."""
    # Alembic читает DATABASE_URL из настроек. Переопределяем переменную,
    # чтобы миграции применились к контейнеру, а не к локальному Postgres.
    previous = os.environ.get("DATABASE_URL")
    os.environ["DATABASE_URL"] = database_url

    from alembic import command
    from alembic.config import Config

    cfg = Config("alembic.ini")
    command.upgrade(cfg, "head")

    try:
        yield
    finally:
        if previous is None:
            os.environ.pop("DATABASE_URL", None)
        else:
            os.environ["DATABASE_URL"] = previous


@pytest.fixture(scope="session")
def engine(database_url: str, alembic_config: None):
    """SQLAlchemy engine, подключённый к контейнеру."""
    eng = create_engine(database_url, pool_pre_ping=True, future=True)
    try:
        yield eng
    finally:
        eng.dispose()


@pytest.fixture()
def session(engine) -> Iterator[Session]:
    """Сессия на один тест. После теста все таблицы очищаются."""
    SessionLocal = sessionmaker(bind=engine, autoflush=False, expire_on_commit=False)
    with SessionLocal() as s:
        try:
            yield s
        finally:
            s.rollback()
            # чистим таблицы между тестами, чтобы они не влияли друг на друга
            s.execute(
                text(
                    "TRUNCATE TABLE task_instance, dag_run, task_dependency, "
                    "task, dag_version, dag, worker RESTART IDENTITY CASCADE"
                )
            )
            s.commit()
