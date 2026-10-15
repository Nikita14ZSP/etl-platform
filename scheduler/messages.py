"""Контракт 1: сообщение планировщика воркеру (TaskMessage v1).

Источник правды по формату — contracts/task_message.v1.schema.json.
Эта модель — обёртка для планировщика: строгая проверка при сборке
сообщения и сериализация в запись Redis Stream. Со стороны воркера то же
самое описано в worker/task/message.go.
"""

from __future__ import annotations

import json
from datetime import date, datetime, timezone
from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field, field_validator

SCHEMA_VERSION = 1

# Значения совпадают с настройками воркера по умолчанию
# (TASK_STREAM и CONSUMER_GROUP в worker/internal/config).
STREAM_NAME = "etl:tasks"
CONSUMER_GROUP = "etl-workers"

# Единственное поле записи стрима, которое читает воркер.
PAYLOAD_FIELD = "payload"

_ID_PATTERN = r"^[A-Za-z0-9_.-]{1,250}$"

Operator = Literal["bash", "python", "sql", "http"]


class TaskContext(BaseModel):
    model_config = ConfigDict(extra="forbid")

    execution_date: date
    xcom: dict[str, Any] = Field(default_factory=dict)


class TaskMessage(BaseModel):
    """Одна попытка выполнения задачи (TaskInstance), поставленная в очередь."""

    # Планировщик — производитель, поэтому строго: опечатка в имени поля
    # должна ловиться при сборке сообщения, а не у воркера. Воркер при этом
    # неизвестные поля игнорирует (см. ADR-0001).
    model_config = ConfigDict(extra="forbid")

    schema_version: Literal[1] = SCHEMA_VERSION
    task_instance_id: str = Field(min_length=1, max_length=128)
    dag_id: str = Field(pattern=_ID_PATTERN)
    run_id: str = Field(min_length=1, max_length=250)
    task_id: str = Field(pattern=_ID_PATTERN)
    try_number: int = Field(ge=1)
    operator: Operator
    config: dict[str, Any]
    context: TaskContext
    timeout_sec: int = Field(ge=1)
    queued_at: datetime

    @field_validator("queued_at")
    @classmethod
    def _queued_at_must_be_utc(cls, value: datetime) -> datetime:
        if value.tzinfo is None or value.utcoffset() != timezone.utc.utcoffset(None):
            raise ValueError("queued_at должен быть timezone-aware и в UTC")
        return value

    def to_json_dict(self) -> dict[str, Any]:
        """Словарь, готовый к json.dumps и совпадающий со схемой контракта."""
        data = self.model_dump(mode="json")
        # pydantic пишет UTC как "+00:00"; в контракте зафиксирован суффикс Z.
        data["queued_at"] = _format_utc(self.queued_at)
        return data

    def to_json(self) -> str:
        return json.dumps(
            self.to_json_dict(), ensure_ascii=False, separators=(",", ":"), sort_keys=True
        )

    def to_stream_fields(self) -> dict[str, str]:
        """Поля для XADD в стрим STREAM_NAME.

        Redis Stream хранит плоские пары строка -> строка, поэтому тело
        целиком лежит в одном поле payload. Версия контракта уже есть
        внутри JSON (schema_version), отдельного поля для неё нет.
        """
        return {PAYLOAD_FIELD: self.to_json()}

    @classmethod
    def from_stream_fields(cls, fields: dict[str, str]) -> "TaskMessage":
        return cls.model_validate_json(fields[PAYLOAD_FIELD])


def _format_utc(moment: datetime) -> str:
    utc = moment.astimezone(timezone.utc)
    text = utc.strftime("%Y-%m-%dT%H:%M:%S")
    if utc.microsecond:
        text += f".{utc.microsecond:06d}".rstrip("0")
    return text + "Z"
