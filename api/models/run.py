from datetime import datetime

from sqlalchemy import (
    CheckConstraint,
    DateTime,
    ForeignKey,
    Index,
    Integer,
    Text,
    UniqueConstraint,
    func,
    text,
)
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.db import Base


class DagRun(Base):
    __tablename__ = "dag_run"
    __table_args__ = (
        UniqueConstraint("run_id", name="uq_dag_run_run_id"),
        CheckConstraint(
            "state IN ('queued', 'running', 'success', 'failed')",
            name="ck_dag_run_state",
        ),
        CheckConstraint(
            "trigger_type IN ('schedule', 'manual', 'backfill')",
            name="ck_dag_run_trigger_type",
        ),
        Index(
            "uq_dag_run_scheduled_dag_exec",
            "dag_id",
            "execution_date",
            unique=True,
            postgresql_where=text("trigger_type = 'schedule'"),
        ),
        Index("ix_dag_run_dag_id_execution_date", "dag_id", "execution_date"),
        Index("ix_dag_run_state", "state"),
    )

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    run_id: Mapped[str] = mapped_column(Text, nullable=False)
    dag_id: Mapped[int] = mapped_column(
        ForeignKey("dag.id", ondelete="CASCADE"),
        nullable=False,
    )
    dag_version_id: Mapped[int] = mapped_column(
        ForeignKey("dag_version.id"),
        nullable=False,
    )
    execution_date: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    state: Mapped[str] = mapped_column(Text, nullable=False, default="queued", server_default="queued")
    trigger_type: Mapped[str] = mapped_column(Text, nullable=False)
    # TODO: confirm with A and B — FK на user(id) добавим в миграции 3.
    triggered_by: Mapped[int | None] = mapped_column(Integer, nullable=True)
    started_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    finished_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now(), onupdate=func.now())

    dag: Mapped["Dag"] = relationship(back_populates="runs")
    dag_version: Mapped["DagVersion"] = relationship(back_populates="runs")
    task_instances: Mapped[list["TaskInstance"]] = relationship(
        back_populates="run",
        cascade="all, delete-orphan",
    )


class TaskInstance(Base):
    __tablename__ = "task_instance"
    __table_args__ = (
        UniqueConstraint("run_id", "task_id", name="uq_task_instance_run_id_task_id"),
        CheckConstraint(
            "state IN ('pending', 'queued', 'running', 'success', 'failed', "
            "'up_for_retry', 'timeout', 'skipped', 'zombie')",
            name="ck_task_instance_state",
        ),
        CheckConstraint("try_number >= 0", name="ck_task_instance_try_number_non_negative"),
        CheckConstraint("max_tries >= 0", name="ck_task_instance_max_tries_non_negative"),
        Index("ix_task_instance_state", "state"),
        Index("ix_task_instance_run_id_state", "run_id", "state"),
        Index(
            "ix_task_instance_running_heartbeat",
            "last_heartbeat",
            postgresql_where=text("state = 'running'"),
        ),
        Index(
            "ix_task_instance_next_retry_at",
            "next_retry_at",
            postgresql_where=text("state = 'up_for_retry'"),
        ),
    )

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    run_id: Mapped[int] = mapped_column(
        ForeignKey("dag_run.id", ondelete="CASCADE"),
        nullable=False,
    )
    task_id: Mapped[int] = mapped_column(
        ForeignKey("task.id"),
        nullable=False,
    )
    state: Mapped[str] = mapped_column(Text, nullable=False, default="pending", server_default="pending")
    try_number: Mapped[int] = mapped_column(Integer, nullable=False, default=0, server_default="0")
    max_tries: Mapped[int] = mapped_column(Integer, nullable=False)
    worker_id: Mapped[int | None] = mapped_column(
        ForeignKey("worker.id"),
        nullable=True,
    )
    queued_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    started_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    finished_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    last_heartbeat: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    next_retry_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    exit_code: Mapped[int | None] = mapped_column(Integer, nullable=True)
    error: Mapped[str | None] = mapped_column(Text, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now(), onupdate=func.now())

    run: Mapped["DagRun"] = relationship(back_populates="task_instances")
    task: Mapped["Task"] = relationship(back_populates="instances")
    worker: Mapped["Worker | None"] = relationship(back_populates="task_instances")
