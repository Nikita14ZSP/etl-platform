from typing import Any

from sqlalchemy import (
    CheckConstraint,
    ForeignKey,
    ForeignKeyConstraint,
    Integer,
    Text,
    UniqueConstraint,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.db import Base


class Task(Base):
    __tablename__ = "task"
    __table_args__ = (
        UniqueConstraint("dag_version_id", "task_id", name="uq_task_dag_version_id_task_id"),
        CheckConstraint(
            "operator IN ('bash', 'python', 'sql', 'http')",
            name="ck_task_operator",
        ),
        CheckConstraint(
            "trigger_rule IN ('all_success', 'all_done', 'all_failed')",
            name="ck_task_trigger_rule",
        ),
        CheckConstraint("retries >= 0", name="ck_task_retries_non_negative"),
        CheckConstraint("retry_delay_sec >= 0", name="ck_task_retry_delay_sec_non_negative"),
        CheckConstraint("timeout_sec > 0", name="ck_task_timeout_sec_positive"),
    )

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    dag_version_id: Mapped[int] = mapped_column(
        ForeignKey("dag_version.id", ondelete="CASCADE"),
        nullable=False,
    )
    task_id: Mapped[str] = mapped_column(Text, nullable=False)
    operator: Mapped[str] = mapped_column(Text, nullable=False)
    config: Mapped[dict[str, Any]] = mapped_column(JSONB, nullable=False, default=dict, server_default="{}")
    retries: Mapped[int] = mapped_column(Integer, nullable=False, default=0, server_default="0")
    retry_delay_sec: Mapped[int] = mapped_column(Integer, nullable=False, default=300, server_default="300")
    timeout_sec: Mapped[int] = mapped_column(Integer, nullable=False, default=1800, server_default="1800")
    trigger_rule: Mapped[str] = mapped_column(Text, nullable=False, default="all_success", server_default="all_success")
    pool: Mapped[str | None] = mapped_column(Text, nullable=True)

    dag_version: Mapped["DagVersion"] = relationship(back_populates="tasks")
    dependencies: Mapped[list["TaskDependency"]] = relationship(
        back_populates="task",
        foreign_keys="TaskDependency.task_id",
        cascade="all, delete-orphan",
    )
    dependents: Mapped[list["TaskDependency"]] = relationship(
        back_populates="depends_on_task",
        foreign_keys="TaskDependency.depends_on_task_id",
    )
    instances: Mapped[list["TaskInstance"]] = relationship(back_populates="task")


class TaskDependency(Base):
    __tablename__ = "task_dependency"
    __table_args__ = (
        ForeignKeyConstraint(
            ["dag_version_id", "task_id"],
            ["task.dag_version_id", "task.task_id"],
            name="fk_task_dependency_task",
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ["dag_version_id", "depends_on_task_id"],
            ["task.dag_version_id", "task.task_id"],
            name="fk_task_dependency_depends_on",
            ondelete="CASCADE",
        ),
        CheckConstraint("task_id <> depends_on_task_id", name="ck_task_dependency_no_self"),
    )

    dag_version_id: Mapped[int] = mapped_column(Integer, primary_key=True, nullable=False)
    task_id: Mapped[str] = mapped_column(Text, primary_key=True, nullable=False)
    depends_on_task_id: Mapped[str] = mapped_column(Text, primary_key=True, nullable=False)

    task: Mapped["Task"] = relationship(
        back_populates="dependencies",
        foreign_keys=[dag_version_id, task_id],
    )
    depends_on_task: Mapped["Task"] = relationship(
        back_populates="dependents",
        foreign_keys=[dag_version_id, depends_on_task_id],
    )
