import React, { useState } from "react";
import { Button } from "@mui/material";
import { ContentCopy, Check } from "@mui/icons-material";
import { PageHeader } from "@/shared/components";

interface AlertDetailHeaderProps {
  title: string;
  checkType: string;
  tenantId: string;
}

export const AlertDetailHeader: React.FC<AlertDetailHeaderProps> = ({
  title,
  checkType,
}) => {
  const [copied, setCopied] = useState(false);

  const handleCopyLink = () => {
    const url = window.location.href;
    navigator.clipboard.writeText(url);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <PageHeader title={title} subtitle={checkType}>
      <Button
        variant="outlined"
        onClick={handleCopyLink}
        startIcon={copied ? <Check /> : <ContentCopy />}
      >
        {copied ? "Copied!" : "Copy Link"}
      </Button>
    </PageHeader>
  );
};
