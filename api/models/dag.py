from datetime import datetime
from typing import Any

from sqlalchemy import (
    Boolean,
    CheckConstraint,
    DateTime,
    ForeignKey,
    Integer,
    Text,
    UniqueConstraint,
    func,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.db import Base


class Dag(Base):
    __tablename__ = "dag"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    dag_id: Mapped[str] = mapped_column(Text, nullable=False, unique=True)
    owner: Mapped[str] = mapped_column(Text, nullable=False)
    is_paused: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False, server_default="false")
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now(), onupdate=func.now())

    versions: Mapped[list["DagVersion"]] = relationship(
        back_populates="dag",
        cascade="all, delete-orphan",
    )
    runs: Mapped[list["DagRun"]] = relationship(
        back_populates="dag",
        cascade="all, delete-orphan",
    )


class DagVersion(Base):
    __tablename__ = "dag_version"
    __table_args__ = (
        UniqueConstraint("dag_id", "version", name="uq_dag_version_dag_id_version"),
        CheckConstraint("version > 0", name="ck_dag_version_version_positive"),
        CheckConstraint("max_active_runs > 0", name="ck_dag_version_max_active_runs_positive"),
    )

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    dag_id: Mapped[int] = mapped_column(
        ForeignKey("dag.id", ondelete="CASCADE"),
        nullable=False,
    )
    version: Mapped[int] = mapped_column(Integer, nullable=False)
    schedule: Mapped[str] = mapped_column(Text, nullable=False)
    start_date: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    catchup: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False, server_default="false")
    max_active_runs: Mapped[int] = mapped_column(Integer, nullable=False, default=1, server_default="1")
    default_args: Mapped[dict[str, Any]] = mapped_column(JSONB, nullable=False, default=dict, server_default="{}")
    source_yaml: Mapped[str] = mapped_column(Text, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())

    dag: Mapped["Dag"] = relationship(back_populates="versions")
    tasks: Mapped[list["Task"]] = relationship(
        back_populates="dag_version",
        cascade="all, delete-orphan",
    )
    runs: Mapped[list["DagRun"]] = relationship(back_populates="dag_version")
