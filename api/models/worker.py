from datetime import datetime

from sqlalchemy import CheckConstraint, DateTime, Integer, Text, func
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.db import Base


class Worker(Base):
    __tablename__ = "worker"
    __table_args__ = (
        CheckConstraint(
            "status IN ('online', 'draining', 'offline')",
            name="ck_worker_status",
        ),
    )

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    instance_id: Mapped[str] = mapped_column(Text, nullable=False, unique=True)
    hostname: Mapped[str] = mapped_column(Text, nullable=False)
    version: Mapped[str] = mapped_column(Text, nullable=False)
    status: Mapped[str] = mapped_column(Text, nullable=False, default="online", server_default="online")
    last_heartbeat: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    started_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())

    task_instances: Mapped[list["TaskInstance"]] = relationship(back_populates="worker")
